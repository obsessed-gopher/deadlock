package vtex

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/obsessed-gopher/deadlock/internal/gamedata/vpk"
)

// build makes a minimal .vtex_c: resource header, one DATA block with the
// texture header, then the pixel data (mips smallest first).
func build(w, h int, f Format, mips int, extraType uint32, pixels []byte) []byte {
	le := binary.LittleEndian
	data := make([]byte, 64)
	le.PutUint16(data[0:], 1)
	le.PutUint16(data[20:], uint16(w))
	le.PutUint16(data[22:], uint16(h))
	le.PutUint16(data[24:], 1)
	data[26] = byte(f)
	data[27] = byte(mips)
	le.PutUint32(data[32:], 8) // extra entries at 40
	le.PutUint32(data[36:], 1)
	le.PutUint32(data[40:], extraType)
	le.PutUint32(data[44:], 8)
	le.PutUint32(data[48:], 4)

	head := make([]byte, 16+12)
	le.PutUint32(head[8:], 8) // block table at 16
	le.PutUint32(head[12:], 1)
	copy(head[16:], "DATA")
	le.PutUint32(head[20:], uint32(len(head)-20)) // DATA right after the table
	le.PutUint32(head[24:], uint32(len(data)))
	file := append(append(head, data...), pixels...)
	le.PutUint32(file[0:], uint32(len(file)))
	return file
}

func TestDecodeBGRAPicksLargestMip(t *testing.T) {
	// 2x2 BGRA with mips 1x1 (4 bytes of junk) then 2x2.
	px := []byte{9, 9, 9, 9}
	px = append(px,
		0, 0, 255, 255, // red
		0, 255, 0, 255, // green
		255, 0, 0, 255, // blue
		10, 20, 30, 0, // transparent
	)
	img, err := Decode(build(2, 2, FormatBGRA8888, 2, 1, px))
	if err != nil {
		t.Fatal(err)
	}
	want := []color.NRGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {30, 20, 10, 0}}
	for i, c := range want {
		if got := img.NRGBAAt(i%2, i/2); got != c {
			t.Errorf("pixel %d = %v, want %v", i, got, c)
		}
	}
}

func TestDecodeDXT1(t *testing.T) {
	// One 4x4 block: c0 = white, c1 = black (c0 > c1: 4-colour mode), all
	// indices 0 except the last pixel -> 1.
	block := []byte{0xff, 0xff, 0, 0, 0, 0, 0, 0x40}
	img, err := Decode(build(4, 4, FormatDXT1, 1, 1, block))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.NRGBAAt(0, 0); got != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("first pixel = %v, want white", got)
	}
	if got := img.NRGBAAt(3, 3); got != (color.NRGBA{0, 0, 0, 255}) {
		t.Errorf("last pixel = %v, want black", got)
	}
}

func TestDecodeDXT5Alpha(t *testing.T) {
	// Alpha endpoints 255 and 0, index 0 everywhere except pixel 15 -> 1.
	alpha := []byte{255, 0, 0, 0, 0, 0, 0, 0x20}
	color := []byte{0xff, 0xff, 0, 0, 0, 0, 0, 0}
	img, err := Decode(build(4, 4, FormatDXT5, 1, 1, append(alpha, color...)))
	if err != nil {
		t.Fatal(err)
	}
	if a := img.NRGBAAt(0, 0).A; a != 255 {
		t.Errorf("alpha[0] = %d, want 255", a)
	}
	if a := img.NRGBAAt(3, 3).A; a != 0 {
		t.Errorf("alpha[15] = %d, want 0", a)
	}
}

func TestDecodePNG(t *testing.T) {
	// Two "mips": a 1x1 PNG, then the 2x1 full-size one.
	enc := func(w int, c color.NRGBA) []byte {
		img := image.NewNRGBA(image.Rect(0, 0, w, 1))
		for x := range w {
			img.SetNRGBA(x, 0, c)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	px := append(enc(1, color.NRGBA{1, 2, 3, 255}), enc(2, color.NRGBA{200, 100, 50, 255})...)
	img, err := Decode(build(2, 1, FormatPNG, 2, 1, px))
	if err != nil {
		t.Fatal(err)
	}
	if img.Rect.Dx() != 2 || img.NRGBAAt(1, 0) != (color.NRGBA{200, 100, 50, 255}) {
		t.Errorf("got %v %v, want the 2x1 mip", img.Rect, img.NRGBAAt(1, 0))
	}
	if _, err := Decode(build(2, 1, FormatPNG, 1, 1, []byte("no png here"))); err == nil {
		t.Error("PNG texture without PNG data: want error")
	}
}

func TestDecodeRejects(t *testing.T) {
	cases := map[string][]byte{
		"unknown format":   build(2, 2, 20, 1, 1, make([]byte, 16)),
		"compressed mips":  build(2, 2, FormatBGRA8888, 1, extraCompressedMipSize, make([]byte, 16)),
		"truncated pixels": build(64, 64, FormatBGRA8888, 1, 1, make([]byte, 16)),
		"not a resource":   []byte("hello"),
	}
	for name, file := range cases {
		if _, err := Decode(file); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	_, err := Decode(build(2, 2, 20, 1, 1, make([]byte, 16)))
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("unknown format err = %v, want ErrUnsupported", err)
	}
}

func TestFit(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for x := range 4 {
		for y := range 2 {
			src.SetNRGBA(x, y, color.NRGBA{200, 100, 0, 255})
		}
	}
	src.SetNRGBA(0, 0, color.NRGBA{0, 0, 0, 0}) // transparent: must not darken
	got := Fit(src, 2)
	if got.Rect.Dx() != 2 || got.Rect.Dy() != 1 {
		t.Fatalf("size = %v, want 2x1", got.Rect)
	}
	if c := got.NRGBAAt(0, 0); c.R != 200 || c.G != 100 || c.A != 191 {
		t.Errorf("averaged pixel = %v, want colour kept, alpha 3/4", c)
	}
	if Fit(src, 8) != src {
		t.Error("small image must be returned as is")
	}
}

// Decodes a real icon when the game is installed (as TestExtractLocalInstall).
func TestDecodeLocalInstall(t *testing.T) {
	dir := os.Getenv("DEADLOCK_GAME_DIR")
	if dir == "" {
		dir = `C:\Program Files (x86)\Steam\steamapps\common\Deadlock\game\citadel`
	}
	pak, err := vpk.Open(filepath.Join(dir, "pak01_dir.vpk"))
	if err != nil {
		t.Skip("Deadlock not installed")
	}
	b, err := pak.ReadFile("panorama/images/heroes/haze_sm_psd.vtex_c")
	if err != nil {
		t.Fatal(err)
	}
	img, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if img.Rect.Dx() != 128 || img.Rect.Dy() != 128 {
		t.Errorf("size = %v, want 128x128", img.Rect)
	}
	var opaque int
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 0 {
			opaque++
		}
	}
	if opaque < 128*128/4 {
		t.Errorf("only %d visible pixels: decoded garbage?", opaque)
	}
}
