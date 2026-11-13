package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DeadlockAPI is a client for the community API at api.deadlock-api.com.
// We use it only for match history and replay salts; replay data itself is
// downloaded from Valve.
type DeadlockAPI struct {
	BaseURL string
	HTTP    *http.Client
	// APIKey, when set, is sent as X-API-Key: deadlock-api gives keys higher
	// limits and a larger share of its queue.
	APIKey string
}

// NewDeadlockAPI returns a client for the public instance.
func NewDeadlockAPI() *DeadlockAPI {
	return &DeadlockAPI{
		BaseURL: "https://api.deadlock-api.com",
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Salts implements SaltProvider. A match the API knows nothing about yields
// ErrNotFound.
func (c *DeadlockAPI) Salts(ctx context.Context, matchID uint64) (Salts, error) {
	var s Salts
	err := c.getJSON(ctx, fmt.Sprintf("/v1/matches/%d/salts", matchID), &s)
	if err == nil && s.MetadataSalt == 0 && s.ReplaySalt == 0 {
		err = fmt.Errorf("deadlock-api: no salts for match %d: %w", matchID, ErrNotFound)
	}
	return s, err
}

// MatchHistory implements HistoryProvider.
func (c *DeadlockAPI) MatchHistory(ctx context.Context, accountID uint32) ([]MatchSummary, error) {
	var out []MatchSummary
	err := c.getJSON(ctx, fmt.Sprintf("/v1/players/%d/match-history", accountID), &out)
	return out, err
}

// MaxProfileBatch is how many account ids /v1/players/steam takes at once.
const MaxProfileBatch = 1000

// SteamProfiles implements ProfileProvider: cached Steam profiles of up to
// MaxProfileBatch accounts in one request. Accounts without a profile are
// simply missing from the result.
func (c *DeadlockAPI) SteamProfiles(ctx context.Context, ids []uint32) ([]SteamProfile, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > MaxProfileBatch {
		return nil, fmt.Errorf("deadlock-api: %d profiles in one request, max %d", len(ids), MaxProfileBatch)
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(uint64(id), 10)
	}
	var out []SteamProfile
	err := c.getJSON(ctx, "/v1/players/steam?account_ids="+strings.Join(parts, ","), &out)
	if errors.Is(err, ErrNotFound) { // none of them has a profile
		return nil, nil
	}
	return out, err
}

// MaxMetaSize caps a downloaded .meta file (real ones are a few MB).
const MaxMetaSize = 64 << 20

// MetadataRaw implements MetaProvider: the compressed .meta file from
// deadlock-api's own storage. disable_steam keeps it off the scarce Steam
// fallback (3 requests an hour per IP): a match it doesn't have is
// ErrNotFound, and the caller tries the salts and Valve's CDN instead.
func (c *DeadlockAPI) MetadataRaw(ctx context.Context, matchID uint64) ([]byte, error) {
	resp, err := c.get(ctx, fmt.Sprintf("/v1/matches/%d/metadata/raw?disable_steam=true", matchID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxMetaSize))
	if err != nil {
		return nil, fmt.Errorf("deadlock-api metadata %d: %w", matchID, err)
	}
	return raw, nil
}

func (c *DeadlockAPI) getJSON(ctx context.Context, path string, v any) error {
	resp, err := c.get(ctx, path)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return json.NewDecoder(resp.Body).Decode(v)
}

// defaultRetryAfter is the pause after a 429 without a Retry-After header.
const defaultRetryAfter = time.Minute

// get returns a 200 response; 404 is ErrNotFound, 429 a *RateLimitError,
// anything else an error with the start of the body.
func (c *DeadlockAPI) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "deadlock-stats/0.1")
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deadlock-api %s: %w", path, err)
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, fmt.Errorf("deadlock-api %s: %w", path, ErrNotFound)
	case http.StatusTooManyRequests:
		return nil, &RateLimitError{What: "deadlock-api " + path, RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return nil, fmt.Errorf("deadlock-api %s: %s: %s", path, resp.Status, body)
}

// retryAfter parses Retry-After in seconds (the HTTP-date form is not used
// by deadlock-api); missing or odd values mean defaultRetryAfter.
func retryAfter(h string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || n <= 0 {
		return defaultRetryAfter
	}
	return time.Duration(n) * time.Second
}
