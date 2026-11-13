package source

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMetadataRaw(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		switch r.URL.Path {
		case "/v1/matches/1/metadata/raw":
			_, _ = w.Write([]byte("BZh91AY&rest"))
		case "/v1/matches/2/metadata/raw":
			w.Header().Set("Retry-After", "42")
			http.Error(w, "slow down", http.StatusTooManyRequests)
		case "/v1/matches/3/metadata/raw":
			http.Error(w, "slow down", http.StatusTooManyRequests)
		case "/v1/matches/4/metadata/raw":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.Error(w, "Match metadata not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := &DeadlockAPI{BaseURL: srv.URL, HTTP: srv.Client()}
	ctx := context.Background()

	raw, err := c.MetadataRaw(ctx, 1)
	if err != nil || string(raw) != "BZh91AY&rest" {
		t.Fatalf("found = %q, %v", raw, err)
	}
	if gotPath != "/v1/matches/1/metadata/raw" || gotQuery != "disable_steam=true" {
		t.Errorf("request = %s?%s; want the Steam fallback disabled", gotPath, gotQuery)
	}
	if _, err := c.MetadataRaw(ctx, 9); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: err = %v, want ErrNotFound", err)
	}
	for id, want := range map[uint64]time.Duration{2: 42 * time.Second, 3: defaultRetryAfter} {
		var rl *RateLimitError
		if _, err := c.MetadataRaw(ctx, id); !errors.As(err, &rl) || rl.RetryAfter != want {
			t.Errorf("429 on %d: err = %v, want retry after %s", id, err, want)
		}
	}
	var rl *RateLimitError
	if _, err := c.MetadataRaw(ctx, 4); err == nil || errors.Is(err, ErrNotFound) || errors.As(err, &rl) {
		t.Errorf("500: err = %v, want a plain error", err)
	}
}

func TestSaltsRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "1200")
		http.Error(w, `{"status":429}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := &DeadlockAPI{BaseURL: srv.URL, HTTP: srv.Client()}
	var rl *RateLimitError
	if _, err := c.Salts(context.Background(), 7); !errors.As(err, &rl) || rl.RetryAfter != 20*time.Minute {
		t.Errorf("salts 429: err = %v, want a rate limit for 20m", err)
	}
}

func TestAPIKeyHeader(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("X-API-Key"))
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()
	c := &DeadlockAPI{BaseURL: srv.URL, HTTP: srv.Client()}
	_, _ = c.MetadataRaw(context.Background(), 1)
	c.APIKey = "k"
	_, _ = c.MetadataRaw(context.Background(), 1)
	if len(got) != 2 || got[0] != "" || got[1] != "k" {
		t.Errorf("X-API-Key headers = %q, want none, then the key", got)
	}
}

func TestRetryAfter(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"5": 5 * time.Second, " 60 ": time.Minute, "": defaultRetryAfter, "0": defaultRetryAfter,
		"-3": defaultRetryAfter, "Wed, 21 Oct 2015 07:28:00 GMT": defaultRetryAfter,
	} {
		if got := retryAfter(in); got != want {
			t.Errorf("retryAfter(%q) = %s, want %s", in, got, want)
		}
	}
}
