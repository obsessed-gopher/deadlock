package source

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

// ErrBadVanity is returned for a string that cannot be a Steam custom URL.
var ErrBadVanity = errors.New("not a steam custom url name")

// vanityRe is what Steam allows in steamcommunity.com/id/<name>.
var vanityRe = regexp.MustCompile(`^[A-Za-z0-9_-]{2,64}$`)

// SteamCommunity resolves custom profile URLs (steamcommunity.com/id/<name>)
// through the public profile XML, which needs no API key. One request per
// user search: no background traffic.
type SteamCommunity struct {
	BaseURL string
	HTTP    *http.Client
}

// NewSteamCommunity returns a client for steamcommunity.com.
func NewSteamCommunity() *SteamCommunity {
	return &SteamCommunity{
		BaseURL: "https://steamcommunity.com",
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

// ResolveVanity returns the account id behind a custom URL name. An unknown
// name yields ErrNotFound, an impossible one ErrBadVanity.
func (c *SteamCommunity) ResolveVanity(ctx context.Context, name string) (uint32, error) {
	if !vanityRe.MatchString(name) {
		return 0, fmt.Errorf("%q: %w", name, ErrBadVanity)
	}
	u := c.BaseURL + "/id/" + url.PathEscape(name) + "/?xml=1"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "deadlock-stats/0.1")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("steam vanity %q: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("steam vanity %q: %s", name, resp.Status)
	}
	// A found profile is <profile><steamID64>…; an unknown one is
	// <response><error>…. Both decode into this struct by element name.
	var doc struct {
		SteamID64 string `xml:"steamID64"`
		Error     string `xml:"error"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return 0, fmt.Errorf("steam vanity %q: %w", name, err)
	}
	if doc.SteamID64 == "" {
		return 0, fmt.Errorf("steam vanity %q: %s: %w", name, doc.Error, ErrNotFound)
	}
	id64, err := strconv.ParseUint(doc.SteamID64, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("steam vanity %q: steamID64 %q: %w", name, doc.SteamID64, err)
	}
	id, ok := NormalizeAccountID(id64)
	if !ok {
		return 0, fmt.Errorf("steam vanity %q: steamID64 %d out of range", name, id64)
	}
	return id, nil
}
