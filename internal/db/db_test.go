package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	v1 "github.com/obsessed-gopher/deadlock/gen/go/deadlock/v1"
	"github.com/obsessed-gopher/deadlock/internal/compress"
	"github.com/obsessed-gopher/deadlock/internal/meta"
	"github.com/obsessed-gopher/deadlock/internal/source"
)

// Integration test against a real Postgres. Run with e.g.
//
//	docker compose up -d postgres
//	TEST_DATABASE_URL=postgres://deadlock:deadlock@localhost:5432/deadlock?sslmode=disable go test ./internal/db
//
// It uses a throwaway schema so it never touches application data.
func TestIngestRoundTrip(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()

	admin, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + time.Now().Format("20060102150405")
	if _, err := admin.Pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})

	d, err := Open(ctx, url+"&search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := d.Migrate(ctx); err != nil { // idempotent
		t.Fatal(err)
	}

	const (
		acc   = 1517505834
		match = 134969900
	)
	if err := d.EnsurePlayer(ctx, acc); err != nil {
		t.Fatal(err)
	}
	hist := []source.MatchSummary{{
		MatchID: match, HeroID: 13, StartTime: 1794330963, DurationS: 1728, GameMode: 1, MatchMode: 1,
		Team: 0, Result: 1, Kills: 8, Deaths: 8, Assists: 8, NetWorth: 29130, LastHits: 116, HeroLevel: 29,
	}}
	n, err := d.UpsertHistory(ctx, acc, hist)
	if err != nil || n != 1 {
		t.Fatalf("UpsertHistory = %d, %v", n, err)
	}
	if n, _ := d.UpsertHistory(ctx, acc, hist); n != 0 {
		t.Errorf("second UpsertHistory new = %d, want 0", n)
	}
	ids, err := d.NextPendingMeta(ctx, 10)
	if err != nil || len(ids) != 1 || ids[0] != match {
		t.Fatalf("NextPendingMeta = %v, %v", ids, err)
	}
	if n, err := d.CountPendingMeta(ctx, acc); err != nil || n != 1 {
		t.Errorf("CountPendingMeta = %d, %v; want 1", n, err)
	}
	if n, err := d.CountPendingMeta(ctx, 42); err != nil || n != 0 {
		t.Errorf("CountPendingMeta for another player = %d, %v; want 0", n, err)
	}

	// A sync that was running when the process died must not stay "running".
	if err := d.SetSyncState(ctx, acc, "running", "", false); err != nil {
		t.Fatal(err)
	}
	if n, err := d.ResetInterruptedSyncs(ctx); err != nil || n != 1 {
		t.Errorf("ResetInterruptedSyncs = %d, %v; want 1", n, err)
	}
	if n, err := d.ResetInterruptedSyncs(ctx); err != nil || n != 0 {
		t.Errorf("second ResetInterruptedSyncs = %d, %v; want 0", n, err)
	}
	if p, err := d.GetPlayer(ctx, acc); err != nil || p.SyncState != v1.SyncState_SYNC_STATE_ERROR || p.SyncError == "" {
		t.Errorf("after reset: player = %+v, %v; want error state with a message", p, err)
	}

	f, err := os.Open("../meta/testdata/134969900.meta.zst")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	raw, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	plain, err := compress.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	version, mi, err := meta.DecodeWithVersion(plain)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // re-ingest must replace, not duplicate
		if err := d.StoreMeta(ctx, match, raw, version, mi); err != nil {
			t.Fatal(err)
		}
	}

	p, err := d.GetPlayer(ctx, acc)
	if err != nil {
		t.Fatal(err)
	}
	if p.MatchesTotal != 1 || p.MatchesWithMeta != 1 || p.MatchesPending != 0 || p.Wins != 0 {
		t.Errorf("player = %+v", p)
	}

	rows, err := d.ListPlayerMatches(ctx, acc, 0, 0, 50)
	if err != nil || len(rows) != 1 || rows[0].Won {
		t.Fatalf("ListPlayerMatches = %v, %v", rows, err)
	}

	pm, err := d.GetPlayerMatch(ctx, match, acc)
	if err != nil {
		t.Fatal(err)
	}
	if len(pm.Match.Players) != 12 {
		t.Errorf("players = %d", len(pm.Match.Players))
	}
	if len(pm.Deaths) != 8 {
		t.Errorf("deaths = %d", len(pm.Deaths))
	}
	if len(pm.Timeline) == 0 || pm.Timeline[len(pm.Timeline)-1].NetWorth != 29130 {
		t.Errorf("timeline end = %+v", pm.Timeline)
	}
	if len(pm.NetWorthBySlot) != 12 {
		t.Errorf("net worth timelines = %d", len(pm.NetWorthBySlot))
	}
	if len(pm.Items) != 28 {
		t.Errorf("items = %d", len(pm.Items))
	}

	if _, err := d.GetPlayerMatch(ctx, match, 42); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown player err = %v", err)
	}

	// Hero stats: two more matches, one won on the same hero, one on another.
	more := []source.MatchSummary{
		{
			MatchID: match + 1, HeroID: 13, StartTime: 1794400000, DurationS: 1000, GameMode: 1, MatchMode: 1,
			Team: 1, Result: 1, Kills: 2, Deaths: 1, Assists: 3, NetWorth: 20000, LastHits: 50, HeroLevel: 20,
		},
		{
			MatchID: match + 2, HeroID: 7, StartTime: 1794300000, DurationS: 900, GameMode: 1, MatchMode: 1,
			Team: 0, Result: 1, Kills: 1, Deaths: 5, Assists: 1, NetWorth: 15000, LastHits: 30, HeroLevel: 18,
		},
	}
	if _, err := d.UpsertHistory(ctx, acc, more); err != nil {
		t.Fatal(err)
	}
	for range 2 { // read-only: repeated calls agree
		heroes, err := d.ListPlayerHeroStats(ctx, acc)
		if err != nil {
			t.Fatal(err)
		}
		if len(heroes) != 2 {
			t.Fatalf("hero stats = %v, want 2 heroes", heroes)
		}
		h := heroes[0] // most played first
		if h.HeroId != 13 || h.Matches != 2 || h.Wins != 1 || h.Kills != 10 || h.Deaths != 9 || h.Assists != 11 ||
			h.NetWorth != 49130 || h.LastHits != 166 || h.DurationS != 2728 || h.LastPlayed != 1794400000 {
			t.Errorf("hero 13 = %+v", h)
		}
		if heroes[1].HeroId != 7 || heroes[1].Matches != 1 || heroes[1].Wins != 0 {
			t.Errorf("hero 7 = %+v", heroes[1])
		}
	}
	if heroes, err := d.ListPlayerHeroStats(ctx, 42); err != nil || len(heroes) != 0 {
		t.Errorf("unknown player hero stats = %v, %v; want none", heroes, err)
	}
}
