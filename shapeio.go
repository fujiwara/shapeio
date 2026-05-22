package shapeio

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const burstLimit = 1000 * 1000 * 1000

type Reader struct {
	r       io.Reader
	limiter atomic.Pointer[rate.Limiter]
	ctx     context.Context
}

type Writer struct {
	w       io.Writer
	limiter atomic.Pointer[rate.Limiter]
	ctx     context.Context
}

// NewReader returns a reader that implements io.Reader with rate limiting.
func NewReader(r io.Reader) *Reader {
	return &Reader{
		r:   r,
		ctx: context.Background(),
	}
}

// NewReaderWithContext returns a reader that implements io.Reader with rate limiting.
func NewReaderWithContext(r io.Reader, ctx context.Context) *Reader {
	return &Reader{
		r:   r,
		ctx: ctx,
	}
}

// NewWriter returns a writer that implements io.Writer with rate limiting.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w:   w,
		ctx: context.Background(),
	}
}

// NewWriterWithContext returns a writer that implements io.Writer with rate limiting.
func NewWriterWithContext(w io.Writer, ctx context.Context) *Writer {
	return &Writer{
		w:   w,
		ctx: ctx,
	}
}

// ReadCloser is a rate-limited io.ReadCloser. It embeds *Reader so all
// rate-limit methods (SetRateLimit, SetRateLimitEvery, ...) are available
// directly, and Close delegates to the wrapped io.ReadCloser.
type ReadCloser struct {
	*Reader
	c io.Closer
}

// NewReadCloser returns a ReadCloser that rate-limits reads from rc and
// closes rc on Close. SetRateLimit must still be called to set the rate.
func NewReadCloser(rc io.ReadCloser) *ReadCloser {
	return &ReadCloser{Reader: NewReader(rc), c: rc}
}

// NewReadCloserWithContext is NewReadCloser with a context for WaitN.
func NewReadCloserWithContext(rc io.ReadCloser, ctx context.Context) *ReadCloser {
	return &ReadCloser{Reader: NewReaderWithContext(rc, ctx), c: rc}
}

// Close closes the wrapped io.ReadCloser.
func (rc *ReadCloser) Close() error {
	return rc.c.Close()
}

// WriteCloser is a rate-limited io.WriteCloser. It embeds *Writer so all
// rate-limit methods are available directly, and Close delegates to the
// wrapped io.WriteCloser.
type WriteCloser struct {
	*Writer
	c io.Closer
}

// NewWriteCloser returns a WriteCloser that rate-limits writes to wc and
// closes wc on Close. SetRateLimit must still be called to set the rate.
func NewWriteCloser(wc io.WriteCloser) *WriteCloser {
	return &WriteCloser{Writer: NewWriter(wc), c: wc}
}

// NewWriteCloserWithContext is NewWriteCloser with a context for WaitN.
func NewWriteCloserWithContext(wc io.WriteCloser, ctx context.Context) *WriteCloser {
	return &WriteCloser{Writer: NewWriterWithContext(wc, ctx), c: wc}
}

// Close closes the wrapped io.WriteCloser.
func (wc *WriteCloser) Close() error {
	return wc.c.Close()
}

// SetRateLimit sets rate limit (bytes/sec) to the reader.
//
// SetRateLimit may be called more than once and concurrently with Read to
// change the rate dynamically. On the first call, a new rate limiter is
// created and the initial burst is consumed. Subsequent calls update the
// existing limiter's rate in place. Note that a rate change does not affect
// a Wait that has already started inside an in-flight Read call; the new
// rate takes effect from the next call.
func (s *Reader) SetRateLimit(bytesPerSec float64) {
	if lim := s.limiter.Load(); lim != nil {
		lim.SetLimit(rate.Limit(bytesPerSec))
		return
	}
	newLim := rate.NewLimiter(rate.Limit(bytesPerSec), burstLimit)
	newLim.AllowN(time.Now(), burstLimit) // spend initial burst
	if !s.limiter.CompareAndSwap(nil, newLim) {
		// Another goroutine installed a limiter first; apply the rate to it.
		s.limiter.Load().SetLimit(rate.Limit(bytesPerSec))
	}
}

// SetRateLimitEvery sets rate limit as bytes per the given duration.
//
// It is equivalent to SetRateLimit(float64(bytes) / per.Seconds()) and is
// useful when the rate is more naturally expressed as "N bytes every D"
// (e.g. SetRateLimitEvery(60, time.Minute)). The same concurrency and
// in-flight-Wait semantics as SetRateLimit apply.
//
// Inputs are not validated; the resulting rate follows
// golang.org/x/time/rate semantics (a non-positive per yields an infinite,
// i.e. unlimited, rate; a negative bytes yields a negative rate).
func (s *Reader) SetRateLimitEvery(bytes int64, per time.Duration) {
	s.SetRateLimit(float64(bytes) / per.Seconds())
}

// Read reads bytes into p.
func (s *Reader) Read(p []byte) (int, error) {
	lim := s.limiter.Load()
	if lim == nil {
		return s.r.Read(p)
	}
	n, err := s.r.Read(p)
	if err != nil {
		return n, err
	}
	if err := lim.WaitN(s.ctx, n); err != nil {
		return n, err
	}
	return n, nil
}

// SetRateLimit sets rate limit (bytes/sec) to the writer.
//
// SetRateLimit may be called more than once and concurrently with Write to
// change the rate dynamically. On the first call, a new rate limiter is
// created and the initial burst is consumed. Subsequent calls update the
// existing limiter's rate in place. Note that a rate change does not affect
// a Wait that has already started inside an in-flight Write call; the new
// rate takes effect from the next call.
func (s *Writer) SetRateLimit(bytesPerSec float64) {
	if lim := s.limiter.Load(); lim != nil {
		lim.SetLimit(rate.Limit(bytesPerSec))
		return
	}
	newLim := rate.NewLimiter(rate.Limit(bytesPerSec), burstLimit)
	newLim.AllowN(time.Now(), burstLimit) // spend initial burst
	if !s.limiter.CompareAndSwap(nil, newLim) {
		s.limiter.Load().SetLimit(rate.Limit(bytesPerSec))
	}
}

// SetRateLimitEvery sets rate limit as bytes per the given duration.
//
// It is equivalent to SetRateLimit(float64(bytes) / per.Seconds()) and is
// useful when the rate is more naturally expressed as "N bytes every D"
// (e.g. SetRateLimitEvery(60, time.Minute)). The same concurrency and
// in-flight-Wait semantics as SetRateLimit apply.
//
// Inputs are not validated; the resulting rate follows
// golang.org/x/time/rate semantics (a non-positive per yields an infinite,
// i.e. unlimited, rate; a negative bytes yields a negative rate).
func (s *Writer) SetRateLimitEvery(bytes int64, per time.Duration) {
	s.SetRateLimit(float64(bytes) / per.Seconds())
}

// Write writes bytes from p.
func (s *Writer) Write(p []byte) (int, error) {
	lim := s.limiter.Load()
	if lim == nil {
		return s.w.Write(p)
	}
	n, err := s.w.Write(p)
	if err != nil {
		return n, err
	}
	if err := lim.WaitN(s.ctx, n); err != nil {
		return n, err
	}
	return n, err
}
