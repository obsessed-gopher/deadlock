// Package vpk reads Valve VPK v1/v2 archives (pak01_dir.vpk + pak01_NNN.vpk).
package vpk

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	signature    = 0x55AA1234
	dirArchive   = 0x7FFF
	entryTermTag = 0xFFFF
)

// Entry describes one file inside the archive set.
type Entry struct {
	Path         string // "scripts/heroes.vdata_c"
	CRC          uint32
	Preload      []byte
	ArchiveIndex uint16
	Offset       uint32
	Length       uint32
}

// Size is the full file size (preload + archived part).
func (e *Entry) Size() int { return len(e.Preload) + int(e.Length) }

// Archive is an opened *_dir.vpk.
type Archive struct {
	dirPath   string
	dataStart int64 // where 0x7FFF-index data begins in the dir file
	entries   map[string]*Entry
}

// Open parses the directory tree of a *_dir.vpk file.
func Open(dirPath string) (*Archive, error) {
	f, err := os.Open(dirPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)

	var hdr struct {
		Signature, Version, TreeSize uint32
	}
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("vpk: header: %w", err)
	}
	if hdr.Signature != signature {
		return nil, errors.New("vpk: bad signature")
	}
	headerSize := int64(12)
	switch hdr.Version {
	case 1:
	case 2:
		var rest [4]uint32 // file data, archive md5, other md5, signature section sizes
		if err := binary.Read(r, binary.LittleEndian, &rest); err != nil {
			return nil, err
		}
		headerSize = 28
	default:
		return nil, fmt.Errorf("vpk: unsupported version %d", hdr.Version)
	}

	a := &Archive{
		dirPath:   dirPath,
		dataStart: headerSize + int64(hdr.TreeSize),
		entries:   map[string]*Entry{},
	}
	for {
		ext, err := cstring(r)
		if err != nil {
			return nil, err
		}
		if ext == "" {
			break
		}
		for {
			dir, err := cstring(r)
			if err != nil {
				return nil, err
			}
			if dir == "" {
				break
			}
			for {
				name, err := cstring(r)
				if err != nil {
					return nil, err
				}
				if name == "" {
					break
				}
				e, err := readEntry(r)
				if err != nil {
					return nil, fmt.Errorf("vpk: entry %s/%s.%s: %w", dir, name, ext, err)
				}
				e.Path = joinPath(dir, name, ext)
				a.entries[e.Path] = e
			}
		}
	}
	return a, nil
}

func readEntry(r *bufio.Reader) (*Entry, error) {
	var raw struct {
		CRC          uint32
		PreloadBytes uint16
		ArchiveIndex uint16
		Offset       uint32
		Length       uint32
		Terminator   uint16
	}
	if err := binary.Read(r, binary.LittleEndian, &raw); err != nil {
		return nil, err
	}
	if raw.Terminator != entryTermTag {
		return nil, fmt.Errorf("bad terminator %#x", raw.Terminator)
	}
	e := &Entry{CRC: raw.CRC, ArchiveIndex: raw.ArchiveIndex, Offset: raw.Offset, Length: raw.Length}
	if raw.PreloadBytes > 0 {
		e.Preload = make([]byte, raw.PreloadBytes)
		if _, err := io.ReadFull(r, e.Preload); err != nil {
			return nil, err
		}
	}
	return e, nil
}

func joinPath(dir, name, ext string) string {
	p := name
	if ext != " " {
		p += "." + ext
	}
	if dir != " " {
		p = dir + "/" + p
	}
	return strings.ToLower(p)
}

func cstring(r *bufio.Reader) (string, error) {
	s, err := r.ReadString(0)
	if err != nil {
		return "", fmt.Errorf("vpk: tree: %w", err)
	}
	return s[:len(s)-1], nil
}

// Entry returns the entry for a path like "scripts/heroes.vdata_c". Case and
// slash direction do not matter on any OS: archive paths never contain a
// backslash, and filepath.ToSlash would only convert them on Windows.
func (a *Archive) Entry(path string) (*Entry, bool) {
	e, ok := a.entries[strings.ToLower(strings.ReplaceAll(path, `\`, "/"))]
	return e, ok
}

// Paths lists all files in the archive.
func (a *Archive) Paths() []string {
	out := make([]string, 0, len(a.entries))
	for p := range a.entries {
		out = append(out, p)
	}
	return out
}

// ReadFile returns the full contents of a file.
func (a *Archive) ReadFile(path string) ([]byte, error) {
	e, ok := a.Entry(path)
	if !ok {
		return nil, fmt.Errorf("vpk: %s: not found", path)
	}
	out := make([]byte, 0, e.Size())
	out = append(out, e.Preload...)
	if e.Length == 0 {
		return out, nil
	}

	var (
		file string
		off  = int64(e.Offset)
	)
	if e.ArchiveIndex == dirArchive {
		file, off = a.dirPath, a.dataStart+off
	} else {
		base := strings.TrimSuffix(a.dirPath, "_dir.vpk")
		file = fmt.Sprintf("%s_%03d.vpk", base, e.ArchiveIndex)
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, e.Length)
	if _, err := f.ReadAt(buf, off); err != nil {
		return nil, fmt.Errorf("vpk: %s: %w", path, err)
	}
	return append(out, buf...), nil
}
