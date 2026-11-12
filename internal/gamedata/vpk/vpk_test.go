package vpk

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type testFile struct {
	ext, dir, name string
	preload        []byte
	archive        uint16
	offset, length uint32
}

// writeArchive builds a minimal VPK v2 directory file followed by dirData
// (the payload of entries stored inside the directory file itself).
func writeArchive(t *testing.T, path string, files []testFile, dirData []byte) {
	t.Helper()
	le := binary.LittleEndian
	var tree bytes.Buffer
	for _, f := range files { // one ext/dir group per file is valid, if verbose
		tree.WriteString(f.ext + "\x00" + f.dir + "\x00" + f.name + "\x00")
		_ = binary.Write(&tree, le, uint32(0)) // crc
		_ = binary.Write(&tree, le, uint16(len(f.preload)))
		_ = binary.Write(&tree, le, f.archive)
		_ = binary.Write(&tree, le, f.offset)
		_ = binary.Write(&tree, le, f.length)
		_ = binary.Write(&tree, le, uint16(entryTermTag))
		tree.Write(f.preload)
		tree.WriteString("\x00\x00") // end of names, end of dirs
	}
	tree.WriteByte(0) // end of extensions

	var out bytes.Buffer
	_ = binary.Write(&out, le, []uint32{signature, 2, uint32(tree.Len()), 0, 0, 0, 0})
	out.Write(tree.Bytes())
	out.Write(dirData)
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestArchive(t *testing.T) {
	dir := t.TempDir()
	dirPath := filepath.Join(dir, "pak01_dir.vpk")
	writeArchive(t, dirPath, []testFile{
		{ext: "txt", dir: "scripts", name: "Preloaded", preload: []byte("all in preload")},
		{ext: "txt", dir: "scripts", name: "indir", archive: dirArchive, offset: 2, length: 6},
		{ext: "vdata_c", dir: "scripts/deep", name: "split", preload: []byte("head+"), archive: 3, offset: 4, length: 4},
		{ext: " ", dir: " ", name: "rootfile", preload: []byte("r")},
	}, []byte("..inside.."))
	if err := os.WriteFile(filepath.Join(dir, "pak01_003.vpk"), []byte("0123tail89"), 0o600); err != nil {
		t.Fatal(err)
	}

	a, err := Open(dirPath)
	if err != nil {
		t.Fatal(err)
	}

	paths := a.Paths()
	sort.Strings(paths)
	wantPaths := []string{"rootfile", "scripts/deep/split.vdata_c", "scripts/indir.txt", "scripts/preloaded.txt"}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Errorf("paths = %q, want %q", paths, wantPaths)
	}

	contents := map[string]string{
		"scripts/preloaded.txt":      "all in preload",
		`SCRIPTS\Preloaded.TXT`:      "all in preload", // lookups ignore case and slash direction
		"scripts/indir.txt":          "inside",         // stored in the directory file after the tree
		"scripts/deep/split.vdata_c": "head+tail",      // preload + pak01_003.vpk
		"rootfile":                   "r",              // no directory, no extension
	}
	for path, want := range contents {
		got, err := a.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("ReadFile(%q) = %q, %v; want %q", path, got, err, want)
		}
	}

	if e, ok := a.Entry("scripts/deep/split.vdata_c"); !ok || e.Size() != 9 {
		t.Errorf("Entry = %+v, %v; want size 9", e, ok)
	}
	if _, err := a.ReadFile("scripts/missing.txt"); err == nil {
		t.Error("missing file: want error")
	}
}

func TestReadFileMissingDataArchive(t *testing.T) {
	dirPath := filepath.Join(t.TempDir(), "pak01_dir.vpk")
	writeArchive(t, dirPath, []testFile{{ext: "txt", dir: "a", name: "b", archive: 7, length: 1}}, nil)
	a, err := Open(dirPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReadFile("a/b.txt"); err == nil {
		t.Error("want error when pak01_007.vpk does not exist")
	}
}

func TestOpenRejectsBadInput(t *testing.T) {
	header := func(vals ...uint32) []byte {
		var b bytes.Buffer
		_ = binary.Write(&b, binary.LittleEndian, vals)
		return b.Bytes()
	}
	cases := map[string][]byte{
		"empty":           nil,
		"bad signature":   header(0xdeadbeef, 2, 0, 0, 0, 0, 0),
		"unknown version": header(signature, 9, 0),
		"truncated tree":  append(header(signature, 1, 100), "txt\x00dir\x00name"...),
		"bad terminator": append(header(signature, 1, 100),
			append([]byte("txt\x00dir\x00name\x00"), make([]byte, 18)...)...),
	}
	for name, content := range cases {
		path := filepath.Join(t.TempDir(), "x_dir.vpk")
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := Open(filepath.Join(t.TempDir(), "absent_dir.vpk")); err == nil {
		t.Error("missing file: want error")
	}
}
