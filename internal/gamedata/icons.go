package gamedata

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/obsessed-gopher/deadlock/internal/gamedata/vpk"
	"github.com/obsessed-gopher/deadlock/internal/gamedata/vtex"
)

// Icon kinds: directories of the extracted icons, also used in their URLs.
const (
	IconHeroes    = "heroes"
	IconAbilities = "abilities"
)

// TexturePath maps an image reference from the game data to its compiled
// texture in the VPK: "file://{images}/heroes/haze_sm.psd" ->
// "panorama/images/heroes/haze_sm_psd.vtex_c". Vector images (.svg) and
// unknown references have no texture: ok is false.
func TexturePath(ref string) (path string, ok bool) {
	rel, found := strings.CutPrefix(ref, "file://{images}/")
	if !found {
		return "", false
	}
	ext := filepath.Ext(rel)
	switch ext {
	case ".psd", ".png", ".tga", ".jpg":
	default:
		return "", false
	}
	return "panorama/images/" + strings.TrimSuffix(rel, ext) + "_" + ext[1:] + ".vtex_c", true
}

// IconStats counts the result of ExtractIcons.
type IconStats struct {
	Written, NoImage, Failed int
}

// ExtractIcons writes the icon of every hero and ability that has one as
// <outDir>/<kind>/<key>.png, scaled down to at most size px. Missing or
// undecodable textures are counted, not fatal: a patch may break a few.
func ExtractIcons(gameDir string, d *Data, outDir string, size int) (IconStats, error) {
	var st IconStats
	pak, err := vpk.Open(filepath.Join(gameDir, "pak01_dir.vpk"))
	if err != nil {
		return st, err
	}
	type job struct{ kind, key, ref string }
	var jobs []job
	for _, h := range d.Heroes {
		jobs = append(jobs, job{IconHeroes, h.Key, h.Image})
	}
	for _, a := range d.Abilities {
		jobs = append(jobs, job{IconAbilities, a.Key, a.Image})
	}
	for _, kind := range []string{IconHeroes, IconAbilities} {
		if err := os.MkdirAll(filepath.Join(outDir, kind), 0o755); err != nil {
			return st, err
		}
	}
	var errs []error
	for _, j := range jobs {
		path, ok := TexturePath(j.ref)
		if !ok || !safeKey(j.key) {
			st.NoImage++
			continue
		}
		if err := writeIcon(pak, path, filepath.Join(outDir, j.kind, j.key+".png"), size); err != nil {
			st.Failed++
			errs = append(errs, fmt.Errorf("%s %s: %w", j.kind, j.key, err))
			continue
		}
		st.Written++
	}
	return st, errors.Join(errs...)
}

func writeIcon(pak *vpk.Archive, texture, out string, size int) error {
	b, err := pak.ReadFile(texture)
	if err != nil {
		return err
	}
	img, err := vtex.Decode(b)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, vtex.Fit(img, size)); err != nil {
		return err
	}
	return os.WriteFile(out, buf.Bytes(), 0o644)
}

// safeKey: keys become file names; only the characters game keys use.
func safeKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
