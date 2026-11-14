package db

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/source"
)

// testDB is a migrated throwaway schema (see TestIngestRoundTrip).
func testDB(t *testing.T) *DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := strings.ToLower(t.Name()) + "_" + time.Now().Format("20060102150405")
	if _, err := admin.Pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	d, err := Open(ctx, url+"&search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		d.Close()
		_, _ = admin.Pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestBotBudget(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	init := source.BotBudget{Tokens: 40, RefilledAt: t0}

	// The first use starts from init; kinds and bots are separate.
	var seen source.BotBudget
	if err := d.UpdateBotBudget(ctx, "bot", "salts", init, func(s *source.BotBudget) {
		seen = *s
		s.Tokens = 3
		s.Strikes = 2
		s.PausedUntil = t0.Add(time.Hour)
	}); err != nil {
		t.Fatal(err)
	}
	if seen.Tokens != 40 || !seen.RefilledAt.Equal(t0) || !seen.PausedUntil.IsZero() {
		t.Errorf("first state = %+v, want init", seen)
	}
	if err := d.UpdateBotBudget(ctx, "bot", "salts", init, func(s *source.BotBudget) { seen = *s }); err != nil {
		t.Fatal(err)
	}
	if seen.Tokens != 3 || seen.Strikes != 2 || !seen.PausedUntil.Equal(t0.Add(time.Hour)) {
		t.Errorf("stored state = %+v, want the saved one", seen)
	}
	if err := d.UpdateBotBudget(ctx, "bot", "history", init, func(s *source.BotBudget) { seen = *s }); err != nil {
		t.Fatal(err)
	}
	if seen.Tokens != 40 {
		t.Errorf("another kind = %+v, want init", seen)
	}

	// Concurrent spenders don't lose each other's updates.
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if err := d.UpdateBotBudget(ctx, "bot", "race", init, func(s *source.BotBudget) { s.Tokens-- }); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	_ = d.UpdateBotBudget(ctx, "bot", "race", init, func(s *source.BotBudget) { seen = *s })
	if seen.Tokens != 20 {
		t.Errorf("after 20 concurrent spends: %v tokens, want 20", seen.Tokens)
	}
}

func TestBotLease(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	lease := func(owner string, ttl time.Duration) bool {
		t.Helper()
		ok, err := d.LeaseBot(ctx, "bot", owner, ttl)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !lease("a", time.Minute) {
		t.Fatal("free bot: not leased")
	}
	if !lease("a", time.Minute) {
		t.Error("renewal by the holder refused")
	}
	if lease("b", time.Minute) {
		t.Error("held bot leased by another owner")
	}
	if ok, _ := d.LeaseBot(ctx, "other", "b", time.Minute); !ok {
		t.Error("another bot: not leased")
	}
	// Releasing frees it; a stranger's release does not.
	if err := d.ReleaseBot(ctx, "bot", "b"); err != nil {
		t.Fatal(err)
	}
	if lease("b", time.Minute) {
		t.Error("released by a non-holder")
	}
	if err := d.ReleaseBot(ctx, "bot", "a"); err != nil {
		t.Fatal(err)
	}
	if !lease("b", time.Millisecond) {
		t.Error("released bot: not leased")
	}
	// An expired lease is up for grabs.
	time.Sleep(20 * time.Millisecond)
	if !lease("a", time.Minute) {
		t.Error("expired lease: not taken over")
	}
}
