package db

import (
	"context"
	"os"
	"testing"
	"time"

	"deadlock_ideas/internal/compress"
	"deadlock_ideas/internal/meta"
	"deadlock_ideas/internal/source"
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

	if _, err := d.GetPlayerMatch(ctx, match, 42); err != ErrNotFound {
		t.Errorf("unknown player err = %v", err)
	}
}
