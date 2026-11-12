// Package vtex decodes compiled Source 2 textures (*.vtex_c) into images:
// enough for the panorama icons of heroes, items and abilities.
//
// Layout (after the resource block container): the DATA block holds the
// texture header; the pixel data follows the blocks at the end of the file,
// mip levels from the smallest to the largest. Supported formats are the
// ones Deadlock icons use: BGRA8888 (almost all), RGBA8888, DXT1, DXT5 and
// PNG_RGBA8888 (each mip is a whole PNG file).
// Mips compressed with LZ4 (extra data COMPRESSED_MIP_SIZE) are not.
package vtex

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"

	"github.com/obsessed-gopher/deadlock/internal/gamedata/resource"
)

// Format is VTexFormat from the game.
type Format uint8

// Formats this package decodes.
const (
	FormatDXT1     Format = 1
	FormatDXT5     Format = 2
	FormatRGBA8888 Format = 4
	FormatPNG      Format = 16
	FormatBGRA8888 Format = 28
)

const extraCompressedMipSize = 4

// ErrUnsupported is returned for texture formats or layouts not handled here.
var ErrUnsupported = errors.New("vtex: unsupported texture")

// Header is the part of the DATA block this package uses.
type Header struct {
	Width, Height, Depth int
	Format               Format
	Mips                 int
	compressedMips       bool
}

// header layout: u16 version, u16 flags, f32[4] reflectivity, u16 width,
// u16 height, u16 depth, u8 format, u8 mips, u32 picmip0, u32 extra offset
// (relative to its own field), u32 extra count; extra entries are
// u32 type, u32 offset (relative to its own field), u32 size.
func parseHeader(data []byte) (Header, error) {
	if len(data) < 40 {
		return Header{}, errors.New("vtex: short DATA block")
	}
	le := binary.LittleEndian
	h := Header{
		Width:  int(le.Uint16(data[20:])),
		Height: int(le.Uint16(data[22:])),
		Depth:  int(le.Uint16(data[24:])),
		Format: Format(data[26]),
		Mips:   int(data[27]),
	}
	extraAt := 32 + int(le.Uint32(data[32:]))
	extraCount := int(le.Uint32(data[36:]))
	for i := range extraCount {
		p := extraAt + 12*i
		if p < 0 || p+12 > len(data) {
			return Header{}, errors.New("vtex: extra data out of range")
		}
		if le.Uint32(data[p:]) == extraCompressedMipSize {
			h.compressedMips = true
		}
	}
	if h.Width == 0 || h.Height == 0 || h.Mips == 0 {
		return Header{}, fmt.Errorf("vtex: empty texture %dx%d, %d mips", h.Width, h.Height, h.Mips)
	}
	return h, nil
}

// mipSize is the byte size of one mip level of this format.
func mipSize(f Format, w, h int) (int, error) {
	blocks := ((w + 3) / 4) * ((h + 3) / 4)
	switch f {
	case FormatBGRA8888, FormatRGBA8888:
		return w * h * 4, nil
	case FormatDXT1:
		return blocks * 8, nil
	case FormatDXT5:
		return blocks * 16, nil
	default:
		return 0, fmt.Errorf("%w: format %d", ErrUnsupported, f)
	}
}

// ReadHeader parses the texture header of a .vtex_c file.
func ReadHeader(file []byte) (Header, error) {
	blocks, err := resource.Blocks(file)
	if err != nil {
		return Header{}, err
	}
	data, ok := blocks["DATA"]
	if !ok {
		return Header{}, errors.New("vtex: no DATA block")
	}
	return parseHeader(data)
}

// Decode returns the full-size (largest mip) image of a .vtex_c file.
func Decode(file []byte) (*image.NRGBA, error) {
	h, err := ReadHeader(file)
	if err != nil {
		return nil, err
	}
	if h.Depth > 1 || h.compressedMips {
		return nil, fmt.Errorf("%w: depth %d, compressed mips %v", ErrUnsupported, h.Depth, h.compressedMips)
	}
	if h.Format == FormatPNG {
		return decodePNG(file)
	}
	total := 0
	for i := range h.Mips {
		n, err := mipSize(h.Format, max(1, h.Width>>i), max(1, h.Height>>i))
		if err != nil {
			return nil, err
		}
		total += n
	}
	largest, _ := mipSize(h.Format, h.Width, h.Height)
	if total > len(file) {
		return nil, fmt.Errorf("vtex: %d bytes of pixels expected, file has %d", total, len(file))
	}
	// Mips are stored smallest first, so the full-size one ends the file.
	px := file[len(file)-largest:]

	img := image.NewNRGBA(image.Rect(0, 0, h.Width, h.Height))
	switch h.Format {
	case FormatBGRA8888:
		for i := 0; i < len(px); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = px[i+2], px[i+1], px[i], px[i+3]
		}
	case FormatRGBA8888:
		copy(img.Pix, px)
	case FormatDXT1, FormatDXT5:
		decodeBlocks(img, px, h.Format)
	}
	return img, nil
}

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// decodePNG: mips are whole PNG files, smallest first, so the last one is
// the full-size image.
func decodePNG(file []byte) (*image.NRGBA, error) {
	i := bytes.LastIndex(file, pngSignature)
	if i < 0 {
		return nil, errors.New("vtex: PNG texture without PNG data")
	}
	src, err := png.Decode(bytes.NewReader(file[i:]))
	if err != nil {
		return nil, fmt.Errorf("vtex: %w", err)
	}
	if n, ok := src.(*image.NRGBA); ok {
		return n, nil
	}
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Rect, src, src.Bounds().Min, draw.Src)
	return dst, nil
}

// decodeBlocks decodes BC1 (DXT1) or BC3 (DXT5) 4x4 blocks.
func decodeBlocks(img *image.NRGBA, px []byte, f Format) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	size := 8
	if f == FormatDXT5 {
		size = 16
	}
	bw := (w + 3) / 4
	for b := 0; b*size < len(px); b++ {
		block := px[b*size : b*size+size]
		bx, by := (b%bw)*4, (b/bw)*4
		var alpha [16]uint8
		for i := range alpha {
			alpha[i] = 255
		}
		colorBlock := block
		if f == FormatDXT5 {
			alpha = dxt5Alpha(block[:8])
			colorBlock = block[8:]
		}
		colors := dxtColors(colorBlock, f == FormatDXT1)
		idx := binary.LittleEndian.Uint32(colorBlock[4:])
		for i := range 16 {
			x, y := bx+i%4, by+i/4
			if x >= w || y >= h {
				continue
			}
			c := colors[(idx>>(2*i))&3]
			if f == FormatDXT5 {
				c.A = alpha[i]
			}
			img.SetNRGBA(x, y, c)
		}
	}
}

func rgb565(v uint16) color.NRGBA {
	r, g, b := uint8(v>>11&31), uint8(v>>5&63), uint8(v&31)
	return color.NRGBA{R: r<<3 | r>>2, G: g<<2 | g>>4, B: b<<3 | b>>2, A: 255}
}

func mix(a, b color.NRGBA, wa, wb, d int) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8((int(x)*wa + int(y)*wb) / d) }
	return color.NRGBA{R: f(a.R, b.R), G: f(a.G, b.G), B: f(a.B, b.B), A: 255}
}

// dxtColors is the 4-colour palette of a BC1 block; in DXT1 mode c0 <= c1
// means 3 colours and transparent black.
func dxtColors(block []byte, dxt1 bool) [4]color.NRGBA {
	v0, v1 := binary.LittleEndian.Uint16(block), binary.LittleEndian.Uint16(block[2:])
	c0, c1 := rgb565(v0), rgb565(v1)
	if dxt1 && v0 <= v1 {
		return [4]color.NRGBA{c0, c1, mix(c0, c1, 1, 1, 2), {}}
	}
	return [4]color.NRGBA{c0, c1, mix(c0, c1, 2, 1, 3), mix(c0, c1, 1, 2, 3)}
}

// dxt5Alpha decodes the 8-byte alpha part of a BC3 block.
func dxt5Alpha(block []byte) [16]uint8 {
	a0, a1 := int(block[0]), int(block[1])
	var pal [8]int
	pal[0], pal[1] = a0, a1
	if a0 > a1 {
		for i := 1; i < 7; i++ {
			pal[i+1] = ((7-i)*a0 + i*a1) / 7
		}
	} else {
		for i := 1; i < 5; i++ {
			pal[i+1] = ((5-i)*a0 + i*a1) / 5
		}
		pal[6], pal[7] = 0, 255
	}
	var bits uint64
	for i := 7; i >= 2; i-- {
		bits = bits<<8 | uint64(block[i])
	}
	var out [16]uint8
	for i := range out {
		out[i] = uint8(pal[(bits>>(3*i))&7])
	}
	return out
}

// Fit scales an image down with a box filter so that neither side exceeds
// size; smaller images are returned as is.
func Fit(src *image.NRGBA, size int) *image.NRGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if w <= size && h <= size {
		return src
	}
	nw, nh := size, size
	if w > h {
		nh = max(1, h*size/w)
	} else {
		nw = max(1, w*size/h)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := range nh {
		y0, y1 := y*h/nh, max(y*h/nh+1, (y+1)*h/nh)
		for x := range nw {
			x0, x1 := x*w/nw, max(x*w/nw+1, (x+1)*w/nw)
			// Average with alpha weighting, so transparent pixels do not
			// darken the edges.
			var r, g, b, a, n int
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					c := src.NRGBAAt(src.Rect.Min.X+sx, src.Rect.Min.Y+sy)
					r += int(c.R) * int(c.A)
					g += int(c.G) * int(c.A)
					b += int(c.B) * int(c.A)
					a += int(c.A)
					n++
				}
			}
			if a == 0 {
				continue
			}
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r / a), G: uint8(g / a), B: uint8(b / a), A: uint8(a / n)})
		}
	}
	return dst
}
