// Package compress opens Valve replay artifacts regardless of their actual
// compression. Files on replay*.valve.net are named *.bz2 but are currently
// zstd; we sniff the magic bytes instead of trusting the extension.
package compress

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"io"

	"github.com/obsessed-gopher/deadlock/internal/zstd"
)

var (
	zstdMagic  = []byte{0x28, 0xB5, 0x2F, 0xFD}
	bzip2Magic = []byte("BZh")
)

// NewReader returns a reader that transparently decompresses zstd or bzip2
// input, or passes plain data through unchanged.
func NewReader(r io.Reader) (io.Reader, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	head, err := br.Peek(4)
	if err != nil && err != io.EOF {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(head, zstdMagic):
		return zstd.NewReader(br), nil
	case bytes.HasPrefix(head, bzip2Magic):
		return bzip2.NewReader(br), nil
	default:
		return br, nil
	}
}

// ReadAll decompresses everything from r into memory.
func ReadAll(r io.Reader) ([]byte, error) {
	dr, err := NewReader(r)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(dr)
}
