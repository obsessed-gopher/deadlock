package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/steamgc"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	want := config{
		DatabaseURL:  "postgres://x",
		GRPCAddr:     ":9090",
		HTTPAddr:     ":8080",
		SyncEvery:    30 * time.Minute,
		MetaInterval: 1500 * time.Millisecond,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	cfg, err := loadConfig(env(map[string]string{
		"DATABASE_URL":     "postgres://x",
		"GRPC_ADDR":        "127.0.0.1:1",
		"HTTP_ADDR":        "127.0.0.1:2",
		"SYNC_EVERY":       "1h",
		"META_INTERVAL":    "250ms",
		"SYNC_ACCOUNTS":    " 42, 76561199477771562 ,,",
		"DEADLOCK_API_KEY": " secret ",
		"STEAM_BOT_USER":   " bot1, bot2 ",
		"STEAM_BOT_TOKEN":  "t1,t2",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GRPCAddr != "127.0.0.1:1" || cfg.HTTPAddr != "127.0.0.1:2" ||
		cfg.SyncEvery != time.Hour || cfg.MetaInterval != 250*time.Millisecond || cfg.DeadlockAPIKey != "secret" {
		t.Errorf("got %+v", cfg)
	}
	if want := []steamgc.Account{{Username: "bot1", Token: "t1"}, {Username: "bot2", Token: "t2"}}; !reflect.DeepEqual(cfg.SteamBots, want) {
		t.Errorf("bots = %v, want %v", cfg.SteamBots, want)
	}
	if want := []uint32{42, 1517505834}; !reflect.DeepEqual(cfg.SyncAccounts, want) {
		t.Errorf("accounts = %v, want %v", cfg.SyncAccounts, want)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"no database", map[string]string{}, "DATABASE_URL"},
		{"bad duration", map[string]string{"DATABASE_URL": "x", "SYNC_EVERY": "soon"}, "SYNC_EVERY"},
		{"zero interval", map[string]string{"DATABASE_URL": "x", "META_INTERVAL": "0s"}, "META_INTERVAL"},
		{"negative period", map[string]string{"DATABASE_URL": "x", "SYNC_EVERY": "-1m"}, "SYNC_EVERY"},
		{"bad account", map[string]string{"DATABASE_URL": "x", "SYNC_ACCOUNTS": "1,nope"}, "SYNC_ACCOUNTS"},
		{"bot without token", map[string]string{"DATABASE_URL": "x", "STEAM_BOT_USER": "bot"}, "STEAM_BOT_TOKEN"},
		{"token without bot", map[string]string{"DATABASE_URL": "x", "STEAM_BOT_TOKEN": "t"}, "STEAM_BOT_USER"},
		{"more bots than tokens", map[string]string{"DATABASE_URL": "x", "STEAM_BOT_USER": "a,b", "STEAM_BOT_TOKEN": "t"}, "pairs"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := loadConfig(env(c.env))
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want mention of %s", err, c.wantErr)
			}
		})
	}
}

func TestLoopback(t *testing.T) {
	if got := loopback(":9090"); got != "localhost:9090" {
		t.Errorf("got %s", got)
	}
	if got := loopback("10.0.0.1:9090"); got != "10.0.0.1:9090" {
		t.Errorf("got %s", got)
	}
}
