package shapeio_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/fujiwara/shapeio"
)

var rates = []float64{
	500 * 1024,       // 500KB/sec
	1024 * 1024,      // 1MB/sec
	10 * 1024 * 1024, // 10MB/sec
	50 * 1024 * 1024, // 50MB/sec
}

var srcs = []*bytes.Reader{
	bytes.NewReader(bytes.Repeat([]byte{0}, 64*1024)),   // 64KB
	bytes.NewReader(bytes.Repeat([]byte{1}, 256*1024)),  // 256KB
	bytes.NewReader(bytes.Repeat([]byte{2}, 1024*1024)), // 1MB
}

func ExampleReader() {
	// example for downloading http body with rate limit.
	resp, _ := http.Get("http://example.com")
	defer resp.Body.Close()

	reader := shapeio.NewReader(resp.Body)
	reader.SetRateLimit(1024 * 10) // 10KB/sec
	io.Copy(ioutil.Discard, reader)
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
			sio := shapeio.NewReader(src)
			sio.SetRateLimit(limit)
			start := time.Now()
			n, err := io.Copy(ioutil.Discard, sio)
			elapsed := time.Since(start)
			if err != nil {
				t.Errorf("io.Copy failed %s", err)
			}
			realRate := float64(n) / elapsed.Seconds()
			if realRate > limit {
				t.Errorf("Limit %f but real rate %f", limit, realRate)
			}
			t.Logf(
				"read %s / %s: Real %s/sec Limit %s/sec. (%f %%)",
				humanize.IBytes(uint64(n)),
				elapsed,
				humanize.IBytes(uint64(realRate)),
				humanize.IBytes(uint64(limit)),
				realRate/limit*100,
			)
		}
	}
}

func TestWrite(t *testing.T) {
	for _, src := range srcs {
		for _, limit := range rates {
			src.Seek(0, 0)
			sio := shapeio.NewWriter(ioutil.Discard)
			sio.SetRateLimit(limit)
			start := time.Now()
			n, err := io.Copy(sio, src)
			elapsed := time.Since(start)
			if err != nil {
				t.Errorf("io.Copy failed %s", err)
			}
			realRate := float64(n) / elapsed.Seconds()
			if realRate > limit {
				t.Errorf("Limit %f but real rate %f", limit, realRate)
			}
			t.Logf(
				"write %s / %s: Real %s/sec Limit %s/sec. (%f %%)",
				humanize.IBytes(uint64(n)),
				elapsed,
				humanize.IBytes(uint64(realRate)),
				humanize.IBytes(uint64(limit)),
				realRate/limit*100,
			)
		}
	}
}
