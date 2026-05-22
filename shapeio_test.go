package shapeio_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fujiwara/shapeio"
)

// iBytes formats b as an IEC binary size string (e.g. "1.0 MiB"). It is a
// tiny replacement for humanize.IBytes, kept here so the test suite has no
// external dependency.
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

var rates = []float64{
	500 * 1024,       // 500KB/sec
	1024 * 1024,      // 1MB/sec
	10 * 1024 * 1024, // 10MB/sec
	50 * 1024 * 1024, // 50MB/sec
}

// rateTolerance is the +/- band (as a fraction of the configured limit)
// the measured rate is allowed to fall into. Short transfers at high
// configured rates can drift a couple of percent in either direction due
// to scheduler / timer noise, especially on shared CI runners.
const rateTolerance = 0.05

// minExpectedDuration skips size/limit combinations whose ideal duration
// is too short for the +/- rateTolerance band to be meaningful — the
// fixed per-transfer overhead (goroutine startup, initial burst spend,
// timer resolution) would dominate and force the result outside the
// band regardless of how the limiter behaves.
const minExpectedDuration = 50 * time.Millisecond

var srcs = []*bytes.Reader{
	bytes.NewReader(bytes.Repeat([]byte{0}, 64*1024)),     // 64KB
	bytes.NewReader(bytes.Repeat([]byte{1}, 256*1024)),    // 256KB
	bytes.NewReader(bytes.Repeat([]byte{2}, 1024*1024)),   // 1MB
	bytes.NewReader(bytes.Repeat([]byte{3}, 4*1024*1024)), // 4MB
}

func ExampleReader() {
	// example for downloading http body with rate limit.
	resp, _ := http.Get("http://example.com")
	defer resp.Body.Close()

	reader := shapeio.NewReader(resp.Body)
	reader.SetRateLimit(1024 * 10) // 10KB/sec
	io.Copy(io.Discard, reader)
}

func ExampleWriter() {
	// example for writing file with rate limit.
	src := bytes.NewReader(bytes.Repeat([]byte{0}, 32*1024)) // 32KB
	f, _ := os.Create("/tmp/foo")
	writer := shapeio.NewWriter(f)
	writer.SetRateLimit(1024 * 10) // 10KB/sec
	io.Copy(writer, src)
	f.Close()
}

func TestRead(t *testing.T) {
	for _, src := range srcs {
		for _, limit := range rates {
			src.Seek(0, 0)
			expected := time.Duration(float64(time.Second) * float64(src.Size()) / limit)
			if expected < minExpectedDuration {
				t.Logf("skip read %d bytes @ %.0f bytes/sec: expected %s < %s",
					src.Size(), limit, expected, minExpectedDuration)
				continue
			}
			sio := shapeio.NewReader(src)
			sio.SetRateLimit(limit)
			start := time.Now()
			n, err := io.Copy(io.Discard, sio)
			elapsed := time.Since(start)
			if err != nil {
				t.Error("io.Copy failed", err)
			}
			realRate := float64(n) / elapsed.Seconds()
			if realRate > limit*(1+rateTolerance) || realRate < limit*(1-rateTolerance) {
				t.Errorf("Limit %f but real rate %f (outside +/- %.0f%%)", limit, realRate, rateTolerance*100)
			}
			t.Logf(
				"read %s / %s: Real %s/sec Limit %s/sec. (%f %%)",
				iBytes(n),
				elapsed,
				iBytes(int64(realRate)),
				iBytes(int64(limit)),
				realRate/limit*100,
			)
		}
	}
}

// TestDynamicReadRateLimit verifies that calling SetRateLimit during a read
// updates the rate in place. The reader starts with a slow limit that would
// take well over a second to finish, then is bumped to a fast limit; the
// transfer must complete well before the original ETA.
func TestDynamicReadRateLimit(t *testing.T) {
	const size = 512 * 1024 // 512KB
	src := bytes.NewReader(bytes.Repeat([]byte{0}, size))
	sio := shapeio.NewReader(src)
	sio.SetRateLimit(32 * 1024) // 32KB/sec → would need ~16s to read 512KB

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond)
		sio.SetRateLimit(50 * 1024 * 1024) // 50MB/sec
	}()

	start := time.Now()
	n, err := io.Copy(io.Discard, sio)
	elapsed := time.Since(start)
	wg.Wait()

	if err != nil {
		t.Fatal(err)
	}
	if n != size {
		t.Fatalf("read %d bytes, want %d", n, size)
	}
	if elapsed >= 5*time.Second {
		t.Errorf("expected dynamic rate change to finish well under 5s, took %s", elapsed)
	}
	t.Logf("dynamic read: %s in %s", iBytes(n), elapsed)
}

// TestDynamicWriteRateLimit is the writer counterpart of TestDynamicReadRateLimit.
// The data is written in small chunks so the rate change is observable between
// Write calls (rate.Limiter.SetLimit does not retroactively shorten a Wait
// that is already in progress).
func TestDynamicWriteRateLimit(t *testing.T) {
	const (
		size      = 512 * 1024
		chunkSize = 16 * 1024
	)
	sio := shapeio.NewWriter(io.Discard)
	sio.SetRateLimit(32 * 1024) // 32KB/sec

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(200 * time.Millisecond)
		sio.SetRateLimit(50 * 1024 * 1024)
	}()

	chunk := make([]byte, chunkSize)
	start := time.Now()
	written := 0
	for written < size {
		m, err := sio.Write(chunk)
		written += m
		if err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	wg.Wait()

	if written != size {
		t.Fatalf("wrote %d bytes, want %d", written, size)
	}
	if elapsed >= 5*time.Second {
		t.Errorf("expected dynamic rate change to finish well under 5s, took %s", elapsed)
	}
	t.Logf("dynamic write: %s in %s", iBytes(int64(written)), elapsed)
}

// TestConcurrentSetRateLimitInitial exercises the case where SetRateLimit and
// Read race from the very first call — i.e. before any limiter has been
// installed. With the atomic.Pointer guard this must be race-free under
// -race.
func TestConcurrentSetRateLimitInitial(t *testing.T) {
	src := bytes.NewReader(bytes.Repeat([]byte{0}, 256*1024))
	sio := shapeio.NewReader(src)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			sio.SetRateLimit(10 * 1024 * 1024)
		}
	}()

	if _, err := io.Copy(io.Discard, sio); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}

type closeRecorderReader struct {
	io.Reader
	closed bool
}

func (c *closeRecorderReader) Close() error {
	c.closed = true
	return nil
}

type closeRecorderWriter struct {
	io.Writer
	closed bool
}

func (c *closeRecorderWriter) Close() error {
	c.closed = true
	return nil
}

// TestReadCloser verifies NewReadCloser exposes Read/SetRateLimit and
// delegates Close to the wrapped io.ReadCloser.
func TestReadCloser(t *testing.T) {
	rec := &closeRecorderReader{Reader: bytes.NewReader([]byte("hello"))}
	rc := shapeio.NewReadCloser(rec)
	rc.SetRateLimit(1024 * 1024)

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("read %q, want %q", got, "hello")
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !rec.closed {
		t.Error("Close was not delegated to the wrapped ReadCloser")
	}
}

// TestWriteCloser verifies NewWriteCloser exposes Write/SetRateLimit and
// delegates Close to the wrapped io.WriteCloser.
func TestWriteCloser(t *testing.T) {
	var buf bytes.Buffer
	rec := &closeRecorderWriter{Writer: &buf}
	wc := shapeio.NewWriteCloser(rec)
	wc.SetRateLimit(1024 * 1024)

	if _, err := wc.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello" {
		t.Fatalf("wrote %q, want %q", buf.String(), "hello")
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !rec.closed {
		t.Error("Close was not delegated to the wrapped WriteCloser")
	}
}

// TestSetRateLimitEvery verifies that SetRateLimitEvery(bytes, per) produces
// the same observable rate as SetRateLimit(float64(bytes)/per.Seconds()).
func TestSetRateLimitEvery(t *testing.T) {
	t.Run("Reader", func(t *testing.T) {
		const size = 1024 * 1024
		src := bytes.NewReader(bytes.Repeat([]byte{0}, size))
		sio := shapeio.NewReader(src)
		sio.SetRateLimitEvery(2*1024*1024, time.Second) // 2MB/sec

		start := time.Now()
		n, err := io.Copy(io.Discard, sio)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		realRate := float64(n) / elapsed.Seconds()
		expected := float64(2 * 1024 * 1024)
		if realRate > expected*(1+rateTolerance) || realRate < expected*(1-rateTolerance) {
			t.Errorf("Limit %f but real rate %f (outside +/- %.0f%%)", expected, realRate, rateTolerance*100)
		}
		t.Logf("read %s in %s -> %s/sec", iBytes(n), elapsed, iBytes(int64(realRate)))
	})

	t.Run("Writer", func(t *testing.T) {
		const size = 1024 * 1024
		src := bytes.NewReader(bytes.Repeat([]byte{0}, size))
		sio := shapeio.NewWriter(io.Discard)
		sio.SetRateLimitEvery(2*1024*1024, time.Second)

		start := time.Now()
		n, err := io.Copy(sio, src)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		realRate := float64(n) / elapsed.Seconds()
		expected := float64(2 * 1024 * 1024)
		if realRate > expected*(1+rateTolerance) || realRate < expected*(1-rateTolerance) {
			t.Errorf("Limit %f but real rate %f (outside +/- %.0f%%)", expected, realRate, rateTolerance*100)
		}
		t.Logf("wrote %s in %s -> %s/sec", iBytes(n), elapsed, iBytes(int64(realRate)))
	})
}

func TestWrite(t *testing.T) {
	for _, src := range srcs {
		for _, limit := range rates {
			src.Seek(0, 0)
			expected := time.Duration(float64(time.Second) * float64(src.Size()) / limit)
			if expected < minExpectedDuration {
				t.Logf("skip write %d bytes @ %.0f bytes/sec: expected %s < %s",
					src.Size(), limit, expected, minExpectedDuration)
				continue
			}
			sio := shapeio.NewWriter(io.Discard)
			sio.SetRateLimit(limit)
			start := time.Now()
			n, err := io.Copy(sio, src)
			elapsed := time.Since(start)
			if err != nil {
				t.Error("io.Copy failed", err)
			}
			realRate := float64(n) / elapsed.Seconds()
			if realRate > limit*(1+rateTolerance) || realRate < limit*(1-rateTolerance) {
				t.Errorf("Limit %f but real rate %f (outside +/- %.0f%%)", limit, realRate, rateTolerance*100)
			}
			t.Logf(
				"write %s / %s: Real %s/sec Limit %s/sec. (%f %%)",
				iBytes(n),
				elapsed,
				iBytes(int64(realRate)),
				iBytes(int64(limit)),
				realRate/limit*100,
			)
		}
	}
}
