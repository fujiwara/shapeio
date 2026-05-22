# shapeio

Traffic shaper for Golang io.Reader and io.Writer

```go
import "github.com/fujiwara/shapeio"

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
```

## Usage

#### type Reader

```go
type Reader struct {
}
```


#### func  NewReader

```go
func NewReader(r io.Reader) *Reader
```
NewReader returns a reader that implements io.Reader with rate limiting.

#### func (*Reader) Read

```go
func (s *Reader) Read(p []byte) (int, error)
```
Read reads bytes into p.

#### func (*Reader) SetRateLimit

```go
func (s *Reader) SetRateLimit(l float64)
```
SetRateLimit sets rate limit (bytes/sec) to the reader.

SetRateLimit may be called more than once and concurrently with Read to
change the rate dynamically. On the first call, a new rate limiter is created
and the initial burst is consumed. Subsequent calls update the existing
limiter's rate in place. Note that a rate change does not affect a Wait that
has already started inside an in-flight Read call; the new rate takes effect
from the next call.

#### func (*Reader) SetRateLimitEvery

```go
func (s *Reader) SetRateLimitEvery(bytes int64, per time.Duration)
```
SetRateLimitEvery sets rate limit as bytes per the given duration. It is
equivalent to `SetRateLimit(float64(bytes) / per.Seconds())` and is useful
when the rate is more naturally expressed as "N bytes every D" (e.g.
`SetRateLimitEvery(60, time.Minute)`).

#### type Writer

```go
type Writer struct {
}
```


#### func  NewWriter

```go
func NewWriter(w io.Writer) *Writer
```
NewWriter returns a writer that implements io.Writer with rate limiting.

#### func (*Writer) SetRateLimit

```go
func (s *Writer) SetRateLimit(l float64)
```
SetRateLimit sets rate limit (bytes/sec) to the writer.

SetRateLimit may be called more than once and concurrently with Write to
change the rate dynamically. On the first call, a new rate limiter is created
and the initial burst is consumed. Subsequent calls update the existing
limiter's rate in place. Note that a rate change does not affect a Wait that
has already started inside an in-flight Write call; the new rate takes effect
from the next call.

#### func (*Writer) SetRateLimitEvery

```go
func (s *Writer) SetRateLimitEvery(bytes int64, per time.Duration)
```
SetRateLimitEvery sets rate limit as bytes per the given duration. It is
equivalent to `SetRateLimit(float64(bytes) / per.Seconds())` and is useful
when the rate is more naturally expressed as "N bytes every D" (e.g.
`SetRateLimitEvery(60, time.Minute)`).

#### func (*Writer) Write

```go
func (s *Writer) Write(p []byte) (int, error)
```
Write writes bytes from p.

#### type ReadCloser

```go
type ReadCloser struct {
	*Reader
}
```

ReadCloser is a rate-limited `io.ReadCloser`. It embeds `*Reader` so all
rate-limit methods (`SetRateLimit`, `SetRateLimitEvery`, ...) are available
directly, and `Close` delegates to the wrapped `io.ReadCloser`. Use this when
you want to pass a single value around that owns both the rate-limited read
and the responsibility to close the underlying source.

#### func  NewReadCloser

```go
func NewReadCloser(rc io.ReadCloser) *ReadCloser
```

#### func  NewReadCloserWithContext

```go
func NewReadCloserWithContext(rc io.ReadCloser, ctx context.Context) *ReadCloser
```

#### type WriteCloser

```go
type WriteCloser struct {
	*Writer
}
```

WriteCloser is a rate-limited `io.WriteCloser`. It embeds `*Writer` so all
rate-limit methods are available directly, and `Close` delegates to the
wrapped `io.WriteCloser`.

#### func  NewWriteCloser

```go
func NewWriteCloser(wc io.WriteCloser) *WriteCloser
```

#### func  NewWriteCloserWithContext

```go
func NewWriteCloserWithContext(wc io.WriteCloser, ctx context.Context) *WriteCloser
```

##  License

The MIT License (MIT)

Copyright (c) 2016 FUJIWARA Shunichiro
