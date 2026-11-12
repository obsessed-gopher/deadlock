// Package kv3 decodes Valve's binary KeyValues3 (KV3) format, as found in
// the DATA block of compiled Source 2 resources (*.vdata_c etc.).
//
// Supported: versions 4 and 5 ("\x043VK", "\x053VK"), uncompressed or zstd.
// Format reference: ValveResourceFormat (MIT), BinaryKV3.cs.
package kv3

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/obsessed-gopher/deadlock/internal/zstd"
)

// Object is a KV3 object preserving key order.
type Object struct {
	Keys   []string
	Values []any
}

// Get returns the value for key, or nil.
func (o *Object) Get(key string) any {
	if o == nil {
		return nil
	}
	for i := len(o.Keys) - 1; i >= 0; i-- { // later duplicates win
		if o.Keys[i] == key {
			return o.Values[i]
		}
	}
	return nil
}

// Obj returns a nested object, or nil.
func (o *Object) Obj(key string) *Object { v, _ := o.Get(key).(*Object); return v }

// Str returns a string field, or "".
func (o *Object) Str(key string) string { v, _ := o.Get(key).(string); return v }

// Int returns a numeric field as int64 (0 if absent or not numeric).
func (o *Object) Int(key string) int64 {
	switch v := o.Get(key).(type) {
	case int64:
		return v
	case uint64:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}

// Value types produced by Decode:
//
//	nil, bool, int64, uint64, float64, string, []byte, []any, *Object
//
// Smaller integer/float widths are widened.

const (
	tNull          = 1
	tBool          = 2
	tInt64         = 3
	tUint64        = 4
	tDouble        = 5
	tString        = 6
	tBlob          = 7
	tArray         = 8
	tObject        = 9
	tArrayTyped    = 10
	tInt32         = 11
	tUint32        = 12
	tTrue          = 13
	tFalse         = 14
	tInt64Zero     = 15
	tInt64One      = 16
	tDoubleZero    = 17
	tDoubleOne     = 18
	tFloat         = 19
	tInt16         = 20
	tUint16        = 21
	tInt8          = 22
	tUint8         = 23
	tArrayTypedLen = 24 // typed array, length in 1-byte lane
	tArrayTypedAux = 25 // typed array, elements in auxiliary buffer
	tUint32Zero    = 26
	tUint32One     = 27
)

const (
	compNone = 0
	compLZ4  = 1
	compZstd = 2

	trailer = 0xFFEEDD00
)

type header struct {
	Format        [16]byte
	Compression   uint32
	DictID        uint16
	FrameSize     uint16
	CountBytes1   int32
	CountBytes4   int32
	CountBytes8   int32
	CountTypes    int32
	CountObjects  uint16
	CountArrays   uint16
	SizeTotal     int32
	SizeCompTotal int32
	CountBlocks   int32
	SizeBlobs     int32
	// v4+
	CountBytes2   int32
	SizeBlockComp int32
	// v5+
	SizeBuf1, SizeCompBuf1         int32
	SizeBuf2, SizeCompBuf2         int32
	B1Buf2, B2Buf2, B4Buf2, B8Buf2 int32
	CountNodes, CountObjectsBuf2   int32
	CountArraysBuf2, CountElems    int32
}

// lanes holds the per-width value streams of one buffer.
type lanes struct {
	b1, b2, b4, b8 []byte
}

type decoder struct {
	version  int
	strings  []string
	types    []byte
	objLens  []byte // v5: object member counts
	buf      *lanes // main buffer (strings ids, container lengths, values)
	aux      *lanes // auxiliary buffer (ARRAY_TYPE_AUXILIARY_BUFFER elements)
	blobs    [][]byte
	nextBlob int
	err      error
}

// Decode parses a binary KV3 document and returns its root value
// (normally *Object).
func Decode(data []byte) (any, error) {
	if len(data) < 4 {
		return nil, errors.New("kv3: short input")
	}
	magic := binary.LittleEndian.Uint32(data)
	if magic&0xFFFFFF00 != 0x4B563300 {
		return nil, fmt.Errorf("kv3: unsupported magic %#08x", magic)
	}
	version := int(magic & 0xFF)
	if version < 4 || version > 5 {
		return nil, fmt.Errorf("kv3: unsupported version %d", version)
	}

	r := bytes.NewReader(data[4:])
	var h header
	fixed := []any{
		&h.Format, &h.Compression, &h.DictID, &h.FrameSize, &h.CountBytes1, &h.CountBytes4,
		&h.CountBytes8, &h.CountTypes, &h.CountObjects, &h.CountArrays, &h.SizeTotal, &h.SizeCompTotal,
		&h.CountBlocks, &h.SizeBlobs, &h.CountBytes2, &h.SizeBlockComp,
	}
	if version >= 5 {
		fixed = append(fixed, &h.SizeBuf1, &h.SizeCompBuf1, &h.SizeBuf2, &h.SizeCompBuf2,
			&h.B1Buf2, &h.B2Buf2, &h.B4Buf2, &h.B8Buf2, &h.CountNodes, &h.CountObjectsBuf2,
			&h.CountArraysBuf2, &h.CountElems)
	}
	for _, f := range fixed {
		if err := binary.Read(r, binary.LittleEndian, f); err != nil {
			return nil, fmt.Errorf("kv3: header: %w", err)
		}
	}
	if h.DictID != 0 {
		return nil, fmt.Errorf("kv3: compression dictionaries not supported")
	}
	switch h.Compression {
	case compNone, compZstd:
	case compLZ4:
		return nil, errors.New("kv3: LZ4 compression not supported yet")
	default:
		return nil, fmt.Errorf("kv3: unknown compression %d", h.Compression)
	}
	if version < 5 {
		h.SizeBuf1, h.SizeCompBuf1 = h.SizeTotal, h.SizeCompTotal
	}

	d := &decoder{version: version}

	// Buffer 1: strings (+ everything before v5).
	buf1Size := int(h.SizeBuf1)
	if version < 5 && h.Compression == compZstd {
		buf1Size += int(h.SizeBlobs) // blobs share the frame before v5
	}
	buf1Raw, err := readBuffer(r, h.Compression, buf1Size, int(h.SizeCompBuf1))
	if err != nil {
		return nil, fmt.Errorf("kv3: buffer 1: %w", err)
	}
	buf1 := buf1Raw[:h.SizeBuf1]
	l1, off, err := splitLanes(buf1, 0, int(h.CountBytes1), int(h.CountBytes2), int(h.CountBytes4), int(h.CountBytes8), version < 5)
	if err != nil {
		return nil, err
	}
	if len(l1.b4) < 4 {
		return nil, errors.New("kv3: missing string count")
	}
	nStrings := int(binary.LittleEndian.Uint32(l1.b4))
	l1.b4 = l1.b4[4:]
	d.aux = l1

	var blobSizes []byte
	if version >= 5 {
		if d.strings, l1.b1, err = readStrings(l1.b1, nStrings); err != nil {
			return nil, err
		}
		buf2, err := readBuffer(r, h.Compression, int(h.SizeBuf2), int(h.SizeCompBuf2))
		if err != nil {
			return nil, fmt.Errorf("kv3: buffer 2: %w", err)
		}
		objEnd := int(h.CountObjectsBuf2) * 4
		if objEnd > len(buf2) {
			return nil, errors.New("kv3: object lengths out of range")
		}
		d.objLens = buf2[:objEnd]
		l2, off2, err := splitLanes(buf2, objEnd, int(h.B1Buf2), int(h.B2Buf2), int(h.B4Buf2), int(h.B8Buf2), false)
		if err != nil {
			return nil, err
		}
		d.buf = l2
		if off2+int(h.CountTypes) > len(buf2) {
			return nil, errors.New("kv3: types out of range")
		}
		d.types = buf2[off2 : off2+int(h.CountTypes)]
		blobSizes = buf2[off2+int(h.CountTypes):]
	} else {
		d.buf = l1
		rest := buf1[off:]
		var used int
		d.strings, rest, err = readStrings(rest, nStrings)
		if err != nil {
			return nil, err
		}
		used = len(buf1[off:]) - len(rest)
		typesLen := int(h.CountTypes) - used
		if typesLen < 0 || typesLen > len(rest) {
			return nil, errors.New("kv3: types out of range")
		}
		d.types = rest[:typesLen]
		blobSizes = rest[typesLen:]
	}

	if h.CountBlocks == 0 {
		if len(blobSizes) < 4 || binary.LittleEndian.Uint32(blobSizes) != trailer {
			return nil, errors.New("kv3: missing trailer")
		}
	} else if err := d.readBlobs(r, &h, blobSizes, buf1Raw[h.SizeBuf1:]); err != nil {
		return nil, err
	}

	t := d.readType()
	root := d.value(t, d.buf)
	if d.err != nil {
		return nil, d.err
	}
	return root, nil
}

func (d *decoder) readBlobs(r *bytes.Reader, h *header, sizes []byte, v4ZstdBlobs []byte) error {
	n := int(h.CountBlocks)
	if len(sizes) < n*4+4 {
		return errors.New("kv3: blob sizes out of range")
	}
	if binary.LittleEndian.Uint32(sizes[n*4:]) != trailer {
		return errors.New("kv3: missing blob trailer")
	}
	total := 0
	lens := make([]int, n)
	for i := range lens {
		lens[i] = int(binary.LittleEndian.Uint32(sizes[i*4:]))
		total += lens[i]
	}
	if total != int(h.SizeBlobs) {
		return errors.New("kv3: blob sizes mismatch")
	}

	var all []byte
	switch {
	case h.Compression == compNone:
		all = make([]byte, total)
		if _, err := io.ReadFull(r, all); err != nil {
			return err
		}
	case d.version < 5: // zstd, decompressed together with buffer 1
		all = v4ZstdBlobs
	default:
		comp := int(h.SizeCompTotal - h.SizeCompBuf1 - h.SizeCompBuf2)
		var err error
		if all, err = readBuffer(r, compZstd, total, comp); err != nil {
			return fmt.Errorf("kv3: blobs: %w", err)
		}
	}
	for _, l := range lens {
		d.blobs = append(d.blobs, all[:l])
		all = all[l:]
	}
	return nil
}

func readBuffer(r *bytes.Reader, comp uint32, size, compSize int) ([]byte, error) {
	out := make([]byte, size)
	if size == 0 {
		if comp == compZstd {
			_, err := r.Seek(int64(compSize), io.SeekCurrent)
			return out, err
		}
		return out, nil
	}
	switch comp {
	case compNone:
		_, err := io.ReadFull(r, out)
		return out, err
	case compZstd:
		frame := make([]byte, compSize)
		if _, err := io.ReadFull(r, frame); err != nil {
			return nil, err
		}
		_, err := io.ReadFull(zstd.NewReader(bytes.NewReader(frame)), out)
		return out, err
	}
	return nil, fmt.Errorf("compression %d", comp)
}

func splitLanes(buf []byte, off, n1, n2, n4, n8 int, alignEmpty8 bool) (*lanes, int, error) {
	l := &lanes{}
	take := func(n, align int) ([]byte, error) {
		off = alignUp(off, align)
		if off+n > len(buf) {
			return nil, errors.New("kv3: lane out of range")
		}
		s := buf[off : off+n]
		off += n
		return s, nil
	}
	var err error
	if n1 > 0 {
		if l.b1, err = take(n1, 1); err != nil {
			return nil, 0, err
		}
	}
	if n2 > 0 {
		if l.b2, err = take(n2*2, 2); err != nil {
			return nil, 0, err
		}
	}
	if n4 > 0 {
		if l.b4, err = take(n4*4, 4); err != nil {
			return nil, 0, err
		}
	}
	if n8 > 0 {
		if l.b8, err = take(n8*8, 8); err != nil {
			return nil, 0, err
		}
	} else if alignEmpty8 {
		off = alignUp(off, 8)
	}
	return l, off, nil
}

func alignUp(v, a int) int { return (v + a - 1) &^ (a - 1) }

func readStrings(b []byte, n int) ([]string, []byte, error) {
	out := make([]string, n)
	for i := range out {
		j := bytes.IndexByte(b, 0)
		if j < 0 {
			return nil, nil, errors.New("kv3: unterminated string")
		}
		out[i] = string(b[:j])
		b = b[j+1:]
	}
	return out, b, nil
}

// --- value decoding ---

func (d *decoder) fail(format string, args ...any) {
	if d.err == nil {
		d.err = fmt.Errorf("kv3: "+format, args...)
	}
}

func (d *decoder) readType() byte {
	if len(d.types) == 0 {
		d.fail("types exhausted")
		return tNull
	}
	t := d.types[0]
	d.types = d.types[1:]
	if t&0x80 != 0 { // flag byte follows; we don't need flags
		if len(d.types) > 0 {
			d.types = d.types[1:]
		}
	}
	if t&0x40 != 0 {
		if len(d.types) > 0 {
			d.types = d.types[1:]
		}
	}
	return t & 0x3F
}

func take(d *decoder, lane *[]byte, n int) []byte {
	if len(*lane) < n {
		d.fail("lane exhausted")
		return make([]byte, n)
	}
	s := (*lane)[:n]
	*lane = (*lane)[n:]
	return s
}

func (d *decoder) u8(l *lanes) uint8   { return take(d, &l.b1, 1)[0] }
func (d *decoder) u16(l *lanes) uint16 { return binary.LittleEndian.Uint16(take(d, &l.b2, 2)) }
func (d *decoder) u32(l *lanes) uint32 { return binary.LittleEndian.Uint32(take(d, &l.b4, 4)) }
func (d *decoder) u64(l *lanes) uint64 { return binary.LittleEndian.Uint64(take(d, &l.b8, 8)) }

func (d *decoder) str(id int32) string {
	if id < 0 || int(id) >= len(d.strings) {
		return ""
	}
	return d.strings[id]
}

func (d *decoder) value(t byte, l *lanes) any {
	if d.err != nil {
		return nil
	}
	switch t {
	case tNull:
		return nil
	case tTrue:
		return true
	case tFalse:
		return false
	case tInt64Zero:
		return int64(0)
	case tInt64One:
		return int64(1)
	case tDoubleZero:
		return 0.0
	case tDoubleOne:
		return 1.0
	case tUint32Zero:
		return uint64(0)
	case tUint32One:
		return uint64(1)

	case tBool:
		return d.u8(l) != 0
	case tInt8:
		return int64(int8(d.u8(l)))
	case tUint8:
		return uint64(d.u8(l))
	case tInt16:
		return int64(int16(d.u16(l)))
	case tUint16:
		return uint64(d.u16(l))
	case tInt32:
		return int64(int32(d.u32(l)))
	case tUint32:
		return uint64(d.u32(l))
	case tFloat:
		return float64(math.Float32frombits(d.u32(l)))
	case tInt64:
		return int64(d.u64(l))
	case tUint64:
		return d.u64(l)
	case tDouble:
		return math.Float64frombits(d.u64(l))

	case tString:
		return d.str(int32(d.u32(d.buf)))
	case tBlob:
		if d.nextBlob >= len(d.blobs) {
			d.fail("blob index out of range")
			return nil
		}
		b := d.blobs[d.nextBlob]
		d.nextBlob++
		return b
	case tArray:
		n := int(d.u32(d.buf))
		out := make([]any, 0, n)
		for i := 0; i < n && d.err == nil; i++ {
			out = append(out, d.value(d.readType(), d.buf))
		}
		return out
	case tArrayTyped, tArrayTypedLen, tArrayTypedAux:
		var n int
		if t == tArrayTyped {
			n = int(d.u32(d.buf))
		} else {
			n = int(d.u8(d.buf))
		}
		sub := d.readType()
		el := d.buf
		if t == tArrayTypedAux {
			el = d.aux
		}
		out := make([]any, 0, n)
		for i := 0; i < n && d.err == nil; i++ {
			out = append(out, d.value(sub, el))
		}
		return out
	case tObject:
		var n int
		if d.version >= 5 {
			n = int(int32(binary.LittleEndian.Uint32(take(d, &d.objLens, 4))))
		} else {
			n = int(d.u32(d.buf))
		}
		o := &Object{Keys: make([]string, 0, n), Values: make([]any, 0, n)}
		for i := 0; i < n && d.err == nil; i++ {
			vt := d.readType()
			key := d.str(int32(d.u32(d.buf)))
			o.Keys = append(o.Keys, key)
			o.Values = append(o.Values, d.value(vt, d.buf))
		}
		return o
	}
	d.fail("unknown type %d", t)
	return nil
}
