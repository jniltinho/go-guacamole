package guac

import (
	"errors"
	"io"
)

// Limits applied to guacd output.
const (
	DefaultMaxInstruction = 8 << 20
	readChunk             = 32 << 10
)

// Reader reads whole instructions from a stream, never returning a partial
// instruction.
type Reader struct {
	r       io.Reader
	buf     []byte
	start   int
	end     int
	maxSize int
}

// NewReader wraps r. maxSize bounds a single instruction (0 → default).
func NewReader(r io.Reader, maxSize int) *Reader {
	if maxSize <= 0 {
		maxSize = DefaultMaxInstruction
	}
	return &Reader{r: r, buf: make([]byte, readChunk*2), maxSize: maxSize}
}

// ReadRaw returns the raw bytes of the next complete instruction. The
// returned slice is only valid until the next call.
func (rd *Reader) ReadRaw() ([]byte, error) {
	for {
		if rd.end > rd.start {
			n, _, err := scan(rd.buf[rd.start:rd.end], 0, false)
			if err != nil {
				return nil, err
			}
			if n > 0 {
				out := rd.buf[rd.start : rd.start+n]
				rd.start += n
				return out, nil
			}
			if rd.end-rd.start >= rd.maxSize {
				return nil, ErrTooLarge
			}
		}
		if err := rd.fill(); err != nil {
			return nil, err
		}
	}
}

// Read parses the next instruction.
func (rd *Reader) Read() (Instruction, error) {
	raw, err := rd.ReadRaw()
	if err != nil {
		return Instruction{}, err
	}
	ins, _, err := Parse(raw, 0)
	return ins, err
}

// Buffered reports whether a complete instruction is already buffered.
func (rd *Reader) Buffered() bool {
	if rd.end <= rd.start {
		return false
	}
	n, _, err := scan(rd.buf[rd.start:rd.end], 0, false)
	return n > 0 || err != nil
}

func (rd *Reader) fill() error {
	// compact
	if rd.start > 0 {
		copy(rd.buf, rd.buf[rd.start:rd.end])
		rd.end -= rd.start
		rd.start = 0
	}
	if len(rd.buf)-rd.end < readChunk {
		size := len(rd.buf) * 2
		if size > rd.maxSize+readChunk {
			size = rd.maxSize + readChunk
		}
		if size <= len(rd.buf) {
			return ErrTooLarge
		}
		nb := make([]byte, size)
		copy(nb, rd.buf[:rd.end])
		rd.buf = nb
	}
	n, err := rd.r.Read(rd.buf[rd.end:])
	rd.end += n
	if n > 0 {
		return nil
	}
	if err == nil {
		return io.ErrNoProgress
	}
	if errors.Is(err, io.EOF) && rd.end > rd.start {
		return io.ErrUnexpectedEOF
	}
	return err
}
