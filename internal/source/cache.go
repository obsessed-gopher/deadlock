package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Cache stores raw (still compressed) replay artifacts on disk so each file is
// downloaded from Valve at most once. Valve deletes old replays after a while,
// so the cache is also our long-term archive.
type Cache struct {
	Dir   string
	Salts SaltProvider
	HTTP  *http.Client
}

func NewCache(dir string, salts SaltProvider) *Cache {
	return &Cache{Dir: dir, Salts: salts, HTTP: &http.Client{Timeout: 30 * time.Minute}}
}

// MetadataPath returns a local path to the match's .meta file, downloading it
// if needed.
func (c *Cache) MetadataPath(ctx context.Context, matchID uint64) (string, error) {
	return c.fetch(ctx, matchID, "meta", Salts.MetadataURL)
}

// DemoPath returns a local path to the match's .dem file, downloading it if
// needed.
func (c *Cache) DemoPath(ctx context.Context, matchID uint64) (string, error) {
	return c.fetch(ctx, matchID, "dem", Salts.DemoURL)
}

func (c *Cache) fetch(ctx context.Context, matchID uint64, kind string, urlOf func(Salts) string) (string, error) {
	path := filepath.Join(c.Dir, kind, fmt.Sprintf("%d.%s.zst", matchID, kind))
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	s, err := c.Salts.Salts(ctx, matchID)
	if err != nil {
		return "", err
	}
	if err := c.download(ctx, urlOf(s), path); err != nil {
		return "", err
	}
	return path, nil
}

func (c *Cache) download(ctx context.Context, url, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s (replay may have expired)", url, resp.Status)
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("download %s: %w", url, err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
