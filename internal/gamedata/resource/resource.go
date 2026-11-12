// Package resource reads the block container of compiled Source 2 resource
// files (*_c) and decodes their KV3 DATA block.
package resource

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/obsessed-gopher/deadlock/internal/gamedata/kv3"
)

// Blocks maps block type ("DATA", "RERL", ...) to its raw bytes.
func Blocks(file []byte) (map[string][]byte, error) {
	if len(file) < 16 {
		return nil, errors.New("resource: short file")
	}
	// u32 file size, u16 header version, u16 type version, u32 block offset, u32 block count
	blockOff := 8 + int(binary.LittleEndian.Uint32(file[8:]))
	count := int(binary.LittleEndian.Uint32(file[12:]))
	out := make(map[string][]byte, count)
	for i := 0; i < count; i++ {
		p := blockOff + i*12
		if p+12 > len(file) {
			return nil, errors.New("resource: block table out of range")
		}
		typ := string(file[p : p+4])
		off := p + 4 + int(binary.LittleEndian.Uint32(file[p+4:])) // relative to the offset field
		size := int(binary.LittleEndian.Uint32(file[p+8:]))
		if off < 0 || off+size > len(file) {
			return nil, fmt.Errorf("resource: block %s out of range", typ)
		}
		out[typ] = file[off : off+size]
	}
	return out, nil
}

// DecodeData decodes the DATA block of a compiled resource as KV3.
func DecodeData(file []byte) (*kv3.Object, error) {
	blocks, err := Blocks(file)
	if err != nil {
		return nil, err
	}
	data, ok := blocks["DATA"]
	if !ok {
		return nil, errors.New("resource: no DATA block")
	}
	v, err := kv3.Decode(data)
	if err != nil {
		return nil, err
	}
	obj, ok := v.(*kv3.Object)
	if !ok {
		return nil, fmt.Errorf("resource: DATA root is %T, want object", v)
	}
	return obj, nil
}
