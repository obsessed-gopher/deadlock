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

func (c *DeadlockAPI) getJSON(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "deadlock-stats/0.1")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("deadlock-api %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("deadlock-api %s: %w", path, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("deadlock-api %s: %s: %s", path, resp.Status, body)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
