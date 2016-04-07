package shapeio

import (
	"io"
	"time"
)

type limiter struct {
	bytesPerSec float64
	startedAt   time.Time
	bytes       int64
}

type Reader struct {
	r io.Reader
	l *limiter
}

type Writer struct {
	w io.Writer
	l *limiter
}

// NewReader returns a reader that implements io.Reader with rate limiting.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		r: r,
		l: &limiter{},
	}
}

// NewWriter returns a writer that implements io.Writer with rate limiting.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: w,
		l: &limiter{},
	}
}

// SetRateLimit sets rate limit (bytes/sec) to the reader.
func (s *Reader) SetRateLimit(l float64) {
	s.l.bytesPerSec = l
}

// Read reads bytes into p.
func (s *Reader) Read(p []byte) (int, error) {
	if !s.l.start() {
		return s.r.Read(p)
	}
	return s.l.wait(s.r.Read(p))
}

// SetRateLimit sets rate limit (bytes/sec) to the writer.
func (s *Writer) SetRateLimit(l float64) {
	s.l.bytesPerSec = l
}

// Write writes bytes from p.
func (s *Writer) Write(p []byte) (int, error) {
	if !s.l.start() {
		return s.w.Write(p)
	}
	return s.l.wait(s.w.Write(p))
}

func (l *limiter) start() bool {
	if l.bytesPerSec == 0 {
		return false
	}
	if l.startedAt.IsZero() {
		l.startedAt = time.Now()
	}
	return true
}

func (l *limiter) wait(n int, err error) (int, error) {
	if n == 0 || err != nil {
		return n, err
	}
	elapsed := time.Since(l.startedAt)
	l.bytes += int64(n)
	rate := float64(l.bytes) / elapsed.Seconds()
	if rate < l.bytesPerSec {
		return n, nil
	}
	d := time.Duration(float64(l.bytes)/l.bytesPerSec*float64(time.Second) - float64(elapsed))
	time.Sleep(d)
	// reset shaping window
	l.startedAt = time.Now()
	l.bytes = 0
	return n, nil
}
