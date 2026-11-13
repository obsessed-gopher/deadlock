package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/source"
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
	// SteamBotUser and SteamBotToken log a bot account on to the Deadlock GC
	// (set up with tools/steambot): fresh history and salts without
	// deadlock-api. Both or neither. The token is a secret: never logged.
	SteamBotUser  string
	SteamBotToken string
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
		SteamBotUser:   strings.TrimSpace(getenv("STEAM_BOT_USER")),
		SteamBotToken:  strings.TrimSpace(getenv("STEAM_BOT_TOKEN")),
	}
	if cfg.DatabaseURL == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}
	if (cfg.SteamBotUser == "") != (cfg.SteamBotToken == "") {
		return config{}, errors.New("STEAM_BOT_USER and STEAM_BOT_TOKEN go together (go run ./tools/steambot login)")
	}

	var err error
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
