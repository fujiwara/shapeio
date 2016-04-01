package shapeio

import (
	"io"
	"time"
)

var zeroTime time.Time

type ShapeIOReader struct {
	reader      io.Reader
	bytesPerSec float64
	start       time.Time
	readBytes   int64
}

func NewReader(reader io.Reader) *ShapeIOReader {
	s := &ShapeIOReader{
		reader: reader,
	}
	return s
}

func (s *ShapeIOReader) SetRateLimit(r float64) {
	s.bytesPerSec = r
}

func (s *ShapeIOReader) Read(b []byte) (int, error) {
	if s.bytesPerSec == 0 {
		return s.reader.Read(b)
	}
	if s.start.Equal(zeroTime) {
		s.start = time.Now()
	}
	n, err := s.reader.Read(b)
	if n == 0 || err != nil {
		return n, err
	}
	duration := time.Since(s.start)
	s.readBytes += int64(n)

	rate := float64(s.readBytes) / duration.Seconds()
	if rate < s.bytesPerSec {
		return n, err
	}
	d := time.Duration(float64(s.readBytes)/s.bytesPerSec*float64(time.Second) - float64(duration))
	time.Sleep(d)
	// reset shaping window
	s.start = time.Now()
	s.readBytes = 0
	return n, err
}
