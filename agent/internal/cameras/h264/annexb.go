package h264

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// Reader splits an Annex-B H.264 byte stream into NAL units, returning the
// NAL payload (without the leading 0x000001 / 0x00000001 start code).
type Reader struct {
	br   *bufio.Reader
	buf  []byte
	eofd bool
}

func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReaderSize(r, 1<<20)}
}

// ReadNAL returns the next NAL payload, or io.EOF when the stream ends.
// Implementation: read up to the next start code, return what's between.
func (r *Reader) ReadNAL() ([]byte, error) {
	if r.eofd {
		return nil, io.EOF
	}
	// Skip past the initial start code on first call. The internal buffer
	// state tracks whether we're "inside" a NAL.
	if r.buf == nil {
		if err := r.skipToStart(); err != nil {
			return nil, err
		}
		r.buf = []byte{}
	}

	var payload bytes.Buffer
	for {
		b, err := r.br.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) && payload.Len() > 0 {
				r.eofd = true
				return payload.Bytes(), nil // last NAL; next call returns EOF
			}
			return nil, err
		}
		_ = payload.WriteByte(b)

		// Detect tail start code.
		p := payload.Bytes()
		if hasTrailingStart(p) {
			cut, _ := trimTrailingStart(p)
			return cut, nil
		}
	}
}

func (r *Reader) skipToStart() error {
	var window [4]byte
	for {
		b, err := r.br.ReadByte()
		if err != nil {
			return err
		}
		window[0], window[1], window[2], window[3] = window[1], window[2], window[3], b
		if window[1] == 0 && window[2] == 0 && window[3] == 1 {
			return nil // consumed 3-byte start
		}
		if window[0] == 0 && window[1] == 0 && window[2] == 0 && window[3] == 1 {
			return nil // consumed 4-byte start
		}
	}
}

func hasTrailingStart(p []byte) bool {
	if len(p) < 3 {
		return false
	}
	tail := p[len(p)-3:]
	if tail[0] == 0 && tail[1] == 0 && tail[2] == 1 {
		return true
	}
	if len(p) >= 4 {
		tail4 := p[len(p)-4:]
		if tail4[0] == 0 && tail4[1] == 0 && tail4[2] == 0 && tail4[3] == 1 {
			return true
		}
	}
	return false
}

func trimTrailingStart(p []byte) ([]byte, int) {
	if len(p) >= 4 && bytes.Equal(p[len(p)-4:], []byte{0, 0, 0, 1}) {
		return p[:len(p)-4], 4
	}
	return p[:len(p)-3], 3
}
