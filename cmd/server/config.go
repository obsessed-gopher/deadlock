package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/source"
	"github.com/obsessed-gopher/deadlock/internal/steamgc"
)

// config is everything the server reads from the environment. The variables
// are documented in the package comment in main.go.
type config struct {
	DatabaseURL  string
	GRPCAddr     string
	HTTPAddr     string
	SyncAccounts []uint32
	SyncEvery    time.Duration
	MetaInterval time.Duration
	// DeadlockAPIKey raises deadlock-api limits (Steam fallback of salts:
	// 300 an hour instead of 3). A secret: never logged.
	DeadlockAPIKey string
	// SteamBots log bot accounts on to the Deadlock GC (set up with
	// tools/steambot): fresh history and salts without deadlock-api.
	// STEAM_BOT_USER and STEAM_BOT_TOKEN are comma-separated lists of equal
	// length, one entry per bot. Tokens are secrets: never logged.
	SteamBots []steamgc.Account
}

// parseBots pairs the comma-separated bot logins with their tokens.
func parseBots(users, tokens string) ([]steamgc.Account, error) {
	split := func(s string) []string {
		var out []string
		for p := range strings.SplitSeq(s, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	us, ts := split(users), split(tokens)
	if len(us) != len(ts) {
		return nil, fmt.Errorf("STEAM_BOT_USER has %d entries, STEAM_BOT_TOKEN %d: they go in pairs (go run ./tools/steambot login)", len(us), len(ts))
	}
	var bots []steamgc.Account
	for i := range us {
		bots = append(bots, steamgc.Account{Username: us[i], Token: ts[i]})
	}
	return bots, nil
}

// loadConfig reads and validates configuration; getenv is os.Getenv in
// production and a map lookup in tests.
func loadConfig(getenv func(string) string) (config, error) {
	get := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}

	cfg := config{
		DatabaseURL: getenv("DATABASE_URL"),
		GRPCAddr:    get("GRPC_ADDR", ":9090"),
		HTTPAddr:    get("HTTP_ADDR", ":8080"),

		DeadlockAPIKey: strings.TrimSpace(getenv("DEADLOCK_API_KEY")),
	}
	if cfg.DatabaseURL == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}

	var err error
	if cfg.SteamBots, err = parseBots(getenv("STEAM_BOT_USER"), getenv("STEAM_BOT_TOKEN")); err != nil {
		return config{}, err
	}
	if cfg.SyncEvery, err = positiveDuration(get("SYNC_EVERY", "30m")); err != nil {
		return config{}, fmt.Errorf("SYNC_EVERY: %w", err)
	}
	if cfg.MetaInterval, err = positiveDuration(get("META_INTERVAL", "1.5s")); err != nil {
		return config{}, fmt.Errorf("META_INTERVAL: %w", err)
	}
	for part := range strings.SplitSeq(getenv("SYNC_ACCOUNTS"), ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		id, err := source.ParseAccountID(part)
		if err != nil {
			return config{}, fmt.Errorf("SYNC_ACCOUNTS: %w", err)
		}
		cfg.SyncAccounts = append(cfg.SyncAccounts, id)
	}
	return cfg, nil
}

// positiveDuration rejects zero and negative values: both are used as ticker
// periods, where they would panic.
func positiveDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be positive, got %s", d)
	}
	return d, nil
}

// loopback turns a listen address like ":9090" into one a client can dial.
func loopback(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}
