package compress

import (
	"bytes"
	"encoding/hex"
	"os"
	"testing"
)

// "hello\n" compressed with bzip2 -9.
const helloBzip2 = "425a6839314159265359c1c080e2000001410000100244a00030cd00c3462997177245385090c1c080e2"

const zstdSample = "../meta/testdata/134969900.meta.zst"

func TestReadAllPlain(t *testing.T) {
	for _, in := range []string{"", "hi", "plain text longer than the magic"} {
		got, err := ReadAll(bytes.NewReader([]byte(in)))
		if err != nil || string(got) != in {
			t.Errorf("ReadAll(%q) = %q, %v", in, got, err)
		}
	}
}

func TestReadAllBzip2(t *testing.T) {
	raw, err := hex.DecodeString(helloBzip2)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadAll(bytes.NewReader(raw))
	if err != nil || string(got) != "hello\n" {
		t.Errorf("got %q, %v; want %q", got, err, "hello\n")
	}
}

// Valve serves zstd under a .bz2 name; the content decides, not the name.
func TestReadAllZstd(t *testing.T) {
	raw, err := os.ReadFile(zstdSample)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadAll(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) <= len(raw) {
		t.Errorf("decompressed %d bytes from %d: expected the output to be larger", len(got), len(raw))
	}
}

func TestReadAllCorrupt(t *testing.T) {
	raw, err := os.ReadFile(zstdSample)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAll(bytes.NewReader(raw[:len(raw)/2])); err == nil {
		t.Error("truncated zstd stream: want error")
	}
}
