package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DeadlockAPI is a client for the community API at api.deadlock-api.com.
// We use it only for match history and replay salts; replay data itself is
// downloaded from Valve.
type DeadlockAPI struct {
	BaseURL string
	HTTP    *http.Client
}

func NewDeadlockAPI() *DeadlockAPI {
	return &DeadlockAPI{
		BaseURL: "https://api.deadlock-api.com",
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *DeadlockAPI) Salts(ctx context.Context, matchID uint64) (Salts, error) {
	var s Salts
	err := c.getJSON(ctx, fmt.Sprintf("/v1/matches/%d/salts", matchID), &s)
	if err == nil && s.MetadataSalt == 0 && s.ReplaySalt == 0 {
		err = fmt.Errorf("deadlock-api: no salts for match %d", matchID)
	}
	return s, err
}

func (c *DeadlockAPI) MatchHistory(ctx context.Context, accountID uint32) ([]MatchSummary, error) {
	var out []MatchSummary
	err := c.getJSON(ctx, fmt.Sprintf("/v1/players/%d/match-history", accountID), &out)
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("deadlock-api %s: %s: %s", path, resp.Status, body)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
