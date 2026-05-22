// Command download is an example CLI that downloads a URL to a file with a
// rate limit toggled live by SIGUSR1.
//
// Usage:
//
//	download --default-rate-limit 1M --lower-rate-limit 100k -o out.bin https://example.com/big.bin
//
// While the download is running, send SIGUSR1 to the process to flip between
// the default rate and the lower rate (0 means "pause" — the underlying
// rate.Limiter blocks once its initial burst is consumed):
//
//	kill -USR1 <pid>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fujiwara/shapeio"
)

func main() {
	var (
		out         string
		defaultRate string
		lowerRate   string
	)
	flag.StringVar(&out, "o", "", "output file (required)")
	flag.StringVar(&defaultRate, "default-rate-limit", "", "default rate limit, e.g. 1M, 500k (required)")
	flag.StringVar(&lowerRate, "lower-rate-limit", "0", "lower rate limit, e.g. 100k or 0 for pause")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] URL\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 || out == "" || defaultRate == "" {
		flag.Usage()
		os.Exit(2)
	}

	defaultBps, err := parseRate(defaultRate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --default-rate-limit: %v\n", err)
		os.Exit(2)
	}
	lowerBps, err := parseRate(lowerRate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --lower-rate-limit: %v\n", err)
		os.Exit(2)
	}

	if err := run(flag.Arg(0), out, defaultBps, lowerBps); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(url, out string, defaultBps, lowerBps float64) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	rc := shapeio.NewReadCloserWithContext(resp.Body, ctx)
	rc.SetRateLimit(defaultBps)

	pid := os.Getpid()
	fmt.Fprintf(os.Stderr,
		"PID %d — send 'kill -USR1 %d' to toggle between default (%s/s) and lower (%s/s)\n",
		pid, pid, iBytes(int64(defaultBps)), iBytes(int64(lowerBps)))

	var inLower atomic.Bool
	usr1 := make(chan os.Signal, 1)
	signal.Notify(usr1, syscall.SIGUSR1)
	defer signal.Stop(usr1)
	go func() {
		for range usr1 {
			if inLower.CompareAndSwap(true, false) {
				rc.SetRateLimit(defaultBps)
			} else {
				inLower.Store(true)
				rc.SetRateLimit(lowerBps)
			}
		}
	}()

	pr := newProgress(resp.ContentLength, &inLower, defaultBps, lowerBps)
	defer pr.Done()

	if _, err := io.Copy(io.MultiWriter(f, pr), rc); err != nil {
		return err
	}
	return nil
}

// progress prints a periodically-updated single-line status to stdout.
type progress struct {
	total       int64
	downloaded  atomic.Int64
	inLower     *atomic.Bool
	defaultBps  float64
	lowerBps    float64
	start       time.Time
	stop        chan struct{}
	doneRunning chan struct{}
}

func newProgress(total int64, inLower *atomic.Bool, defaultBps, lowerBps float64) *progress {
	p := &progress{
		total:       total,
		inLower:     inLower,
		defaultBps:  defaultBps,
		lowerBps:    lowerBps,
		start:       time.Now(),
		stop:        make(chan struct{}),
		doneRunning: make(chan struct{}),
	}
	go p.run()
	return p
}

func (p *progress) Write(b []byte) (int, error) {
	p.downloaded.Add(int64(len(b)))
	return len(b), nil
}

func (p *progress) Done() {
	close(p.stop)
	<-p.doneRunning
}

func (p *progress) run() {
	defer close(p.doneRunning)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

	var (
		lastBytes int64
		lastAt    = p.start
	)
	render := func(now time.Time, instRate float64) {
		cur := p.downloaded.Load()
		mode := "default"
		setBps := p.defaultBps
		if p.inLower.Load() {
			mode = "lower"
			setBps = p.lowerBps
		}
		var line strings.Builder
		fmt.Fprintf(&line, "\r%s", iBytes(cur))
		if p.total > 0 {
			pct := float64(cur) / float64(p.total) * 100
			fmt.Fprintf(&line, " / %s (%.1f%%)", iBytes(p.total), pct)
		}
		fmt.Fprintf(&line, " @ %s/s [%s set=%s/s]", iBytes(int64(instRate)), mode, iBytes(int64(setBps)))
		if p.total > 0 && instRate > 1 {
			remain := float64(p.total-cur) / instRate
			if remain >= 0 {
				fmt.Fprintf(&line, " ETA %s", time.Duration(remain*float64(time.Second)).Truncate(time.Second))
			}
		}
		// Pad with spaces to overwrite any previous longer line.
		line.WriteString("        ")
		fmt.Print(line.String())
	}

	for {
		select {
		case <-p.stop:
			now := time.Now()
			dt := now.Sub(lastAt).Seconds()
			rate := 0.0
			if dt > 0 {
				rate = float64(p.downloaded.Load()-lastBytes) / dt
			}
			render(now, rate)
			fmt.Println()
			return
		case now := <-tick.C:
			cur := p.downloaded.Load()
			dt := now.Sub(lastAt).Seconds()
			rate := 0.0
			if dt > 0 {
				rate = float64(cur-lastBytes) / dt
			}
			render(now, rate)
			lastBytes = cur
			lastAt = now
		}
	}
}

// parseRate parses values like "1M", "500k", "2G", "1024" into bytes/sec.
// An empty or "0" value is treated as 0 (pause).
func parseRate(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty value")
	}
	mult := 1.0
	switch last := s[len(s)-1]; last {
	case 'k', 'K':
		mult = 1024
		s = s[:len(s)-1]
	case 'm', 'M':
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case 'g', 'G':
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("not a number: %q", s)
	}
	if v < 0 {
		return 0, fmt.Errorf("must be non-negative: %q", s)
	}
	return v * mult, nil
}

// iBytes formats b as an IEC binary size string (e.g. "1.0 MiB").
func iBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
