# shapeio

Traffic shaper for Golang io.Reader and io.Writer

```go
import "github.com/fujiwara/shapeio"

resp, _ := http.Get("http://example.com/big.bin")
// ReadCloser bundles the rate-limited Read with Close on the underlying body.
rc := shapeio.NewReadCloser(resp.Body)
defer rc.Close()

// "N bytes per duration" reads naturally; equivalent to SetRateLimit(1<<20).
rc.SetRateLimitEvery(1*1024*1024, time.Second) // 1 MiB/sec

// SetRateLimit is safe to call again at any time, even concurrently with Read.
// Pass 0 to pause; pass any value to resume.
go func() {
	time.Sleep(2 * time.Second)
	rc.SetRateLimit(0)          // pause
	time.Sleep(time.Second)
	rc.SetRateLimit(512 * 1024) // resume at 512 KiB/sec
}()

f, _ := os.Create("/tmp/out.bin")
defer f.Close()
io.Copy(f, rc)
```

`NewWriteCloser` / `SetRateLimitEvery` are mirrored on the writer side.
See `example/download` for a runnable CLI that toggles the rate live via `SIGUSR1`.

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

## Example CLI

`example/download` is a small `curl`-like downloader that demonstrates dynamic
rate switching via `SIGUSR1`. It downloads a URL to a file using two
configured rates and toggles between them every time the process receives
`SIGUSR1`.

```sh
go run ./example/download \
    --default-rate-limit 1M \
    --lower-rate-limit 100k \
    -o /tmp/out.bin \
    https://example.com/big.bin

# In another shell, toggle the rate live:
kill -USR1 <pid>
```

Rate values accept a trailing `k`/`M`/`G` (IEC binary units). A value of `0`
for `--lower-rate-limit` (the default) means "pause" — the underlying limiter
blocks once its initial burst is consumed, and the next `SIGUSR1` resumes
downloading at the default rate.

##  License

The MIT License (MIT)

Copyright (c) 2016 FUJIWARA Shunichiro
