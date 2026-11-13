package db

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	v1 "github.com/obsessed-gopher/deadlock/gen/go/deadlock/v1"
	"github.com/obsessed-gopher/deadlock/internal/compress"
	"github.com/obsessed-gopher/deadlock/internal/meta"
	"github.com/obsessed-gopher/deadlock/internal/pb"
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

	rows, total, err := d.ListPlayerMatches(ctx, MatchQuery{AccountID: acc, Limit: 50})
	if err != nil || len(rows) != 1 || rows[0].Won || total != 1 {
		t.Fatalf("ListPlayerMatches = %v, %d, %v", rows, total, err)
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

	testMatchQueries(ctx, t, d, acc, match)
	testMatchFacts(ctx, t, d, acc, match, mi.GetAverageBadgeTeam0(), mi.GetAverageBadgeTeam1())
	testRederive(ctx, t, d, match, raw, mi)
	testDeferMeta(ctx, t, d, match)
	testPeers(ctx, t, d, acc, match)
	testProfiles(ctx, t, d, acc)
}

// testMatchFacts: the matches of testMatchQueries, oldest first; only match
// has metadata (29 130 souls in a 28:48 game).
func testMatchFacts(ctx context.Context, t *testing.T, d *DB, acc uint32, match uint64, b0, b1 uint32) {
	t.Helper()
	facts, err := d.ListPlayerMatchFacts(ctx, acc)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uint64
	for _, f := range facts {
		ids = append(ids, f.MatchId)
	}
	if !slices.Equal(ids, []uint64{match + 2, match, match + 1}) {
		t.Fatalf("facts order = %v, want oldest first", ids)
	}
	if f := facts[0]; f.HasMeta || f.SoulsAt != nil || f.SoulsFrom != nil || f.LobbyBadge != 0 || f.HeroId != 7 {
		t.Errorf("fact without meta = %+v", f)
	}
	f := facts[1]
	if !f.HasMeta || f.NetWorth != 29130 || f.PlayerDamage == 0 {
		t.Errorf("fact with meta = %+v", f)
	}
	if want := lobbyBadge(int16(b0), int16(b1)); f.LobbyBadge != want {
		t.Errorf("lobby badge = %d, want %d", f.LobbyBadge, want)
	}
	if len(f.SoulsAt) != len(SoulMarksS) || !slices.IsSorted(f.SoulsAt) || f.SoulsAt[0] == 0 {
		t.Errorf("souls at %v = %v, want growing and non-zero", SoulMarksS, f.SoulsAt)
	}
	// 10k and 20k reached within the game, 30k never.
	if tt := f.TimeToSoulsS; len(tt) != 3 || tt[0] == 0 || tt[1] <= tt[0] || tt[1] > 1728 || tt[2] != 0 {
		t.Errorf("time to %v = %v", SoulTargets, tt)
	}
	src := f.SoulsFrom
	if sum := src.GetHeroes() + src.GetLaneCreeps() + src.GetNeutrals() + src.GetBosses() + src.GetTreasure(); sum == 0 || sum > 2*f.NetWorth {
		t.Errorf("soul sources = %+v (sum %d)", src, sum)
	}
	if facts, err := d.ListPlayerMatchFacts(ctx, 42); err != nil || len(facts) != 0 {
		t.Errorf("unknown player facts = %v, %v", facts, err)
	}
}

// testRederive: a match derived by an older version is found, rebuilt from
// meta_raw and meta_raw stays byte for byte the same.
func testRederive(ctx context.Context, t *testing.T, d *DB, match uint64, raw []byte, mi *pb.CMsgMatchMetaDataContents_MatchInfo) {
	t.Helper()
	if ids, err := d.NextStaleMeta(ctx, 0, 10); err != nil || len(ids) != 0 {
		t.Fatalf("fresh ingest is stale: %v, %v", ids, err)
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE matches SET derive_version = 0, badge_team0 = 0 WHERE match_id = $1`, match); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `DELETE FROM deaths WHERE match_id = $1`, match); err != nil {
		t.Fatal(err)
	}
	ids, err := d.NextStaleMeta(ctx, 0, 10)
	if err != nil || !slices.Equal(ids, []uint64{match}) {
		t.Fatalf("NextStaleMeta = %v, %v", ids, err)
	}
	if ids, _ := d.NextStaleMeta(ctx, match, 10); len(ids) != 0 {
		t.Errorf("NextStaleMeta after the last id = %v", ids)
	}
	got, err := d.MetaRaw(ctx, match)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("MetaRaw = %d bytes, %v", len(got), err)
	}
	if err := d.RederiveMeta(ctx, match, mi); err != nil {
		t.Fatal(err)
	}
	if ids, _ := d.NextStaleMeta(ctx, 0, 10); len(ids) != 0 {
		t.Errorf("still stale after rederive: %v", ids)
	}
	pm, err := d.GetPlayerMatch(ctx, match, 1517505834)
	if err != nil || len(pm.Deaths) != 8 {
		t.Errorf("deaths after rederive = %d, %v", len(pm.GetDeaths()), err)
	}
	if got, _ := d.MetaRaw(ctx, match); !bytes.Equal(got, raw) {
		t.Error("rederive changed meta_raw")
	}
	if _, err := d.MetaRaw(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("MetaRaw of unknown match: %v", err)
	}
}

// testMatchQueries runs on the three matches of TestIngestRoundTrip:
// match (hero 13, 8 kills, loss, meta ok), match+1 (hero 13, 2 kills, win)
// and match+2 (hero 7, 1 kill, loss).
func testMatchQueries(ctx context.Context, t *testing.T, d *DB, acc uint32, match uint64) {
	t.Helper()
	ids := func(q MatchQuery) ([]uint64, int) {
		t.Helper()
		rows, total, err := d.ListPlayerMatches(ctx, q)
		if err != nil {
			t.Fatalf("ListPlayerMatches(%+v): %v", q, err)
		}
		out := []uint64{}
		for _, r := range rows {
			out = append(out, r.MatchId)
		}
		return out, total
	}
	// Walk all pages one row at a time, as the server does with tokens.
	walk := func(q MatchQuery) []uint64 {
		t.Helper()
		q.Limit = 1
		var out []uint64
		for range 10 {
			rows, _, err := d.ListPlayerMatches(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) == 0 {
				return out
			}
			out = append(out, rows[0].MatchId)
			q.After = &MatchCursor{Value: MatchSortValue(rows[0], q.Sort), MatchID: rows[0].MatchId}
		}
		t.Fatalf("paging did not end: %v", out)
		return nil
	}
	base := MatchQuery{AccountID: acc, Limit: 50}
	cases := []struct {
		name      string
		q         MatchQuery
		want      []uint64
		wantTotal int
	}{
		{"newest first by default", base, []uint64{match + 1, match, match + 2}, 3},
		{"oldest first", with(base, func(q *MatchQuery) { q.Ascending = true }), []uint64{match + 2, match, match + 1}, 3},
		{"most kills first", with(base, func(q *MatchQuery) { q.Sort = v1.MatchSort_MATCH_SORT_KILLS }), []uint64{match, match + 1, match + 2}, 3},
		{"wins only", with(base, func(q *MatchQuery) { q.Result = v1.MatchResult_MATCH_RESULT_WIN }), []uint64{match + 1}, 1},
		{"losses on hero 13", with(base, func(q *MatchQuery) {
			q.Result, q.HeroID = v1.MatchResult_MATCH_RESULT_LOSS, 13
		}), []uint64{match}, 1},
		{"with metadata", with(base, func(q *MatchQuery) { q.MetaStatus = v1.MetaStatus_META_STATUS_OK }), []uint64{match}, 1},
		{"other game mode", with(base, func(q *MatchQuery) { q.GameMode = 4 }), []uint64{}, 0},
		{"limit does not change total", with(base, func(q *MatchQuery) { q.Limit = 1 }), []uint64{match + 1}, 3},
	}
	for _, c := range cases {
		got, total := ids(c.q)
		if !slices.Equal(got, c.want) || total != c.wantTotal {
			t.Errorf("%s: got %v (total %d), want %v (total %d)", c.name, got, total, c.want, c.wantTotal)
		}
	}
	for _, sort := range []v1.MatchSort{v1.MatchSort_MATCH_SORT_START_TIME, v1.MatchSort_MATCH_SORT_KILLS, v1.MatchSort_MATCH_SORT_DURATION} {
		for _, asc := range []bool{false, true} {
			q := with(base, func(q *MatchQuery) { q.Sort, q.Ascending = sort, asc })
			all, _ := ids(q)
			if paged := walk(q); !slices.Equal(paged, all) {
				t.Errorf("sort %v asc=%v: pages %v, want %v", sort, asc, paged, all)
			}
		}
	}
	if _, _, err := d.ListPlayerMatches(ctx, with(base, func(q *MatchQuery) { q.Sort = 99 })); !errors.Is(err, ErrBadQuery) {
		t.Errorf("unknown sort err = %v, want ErrBadQuery", err)
	}
	if _, _, err := d.ListPlayerMatches(ctx, with(base, func(q *MatchQuery) { q.Result = 5 })); !errors.Is(err, ErrBadQuery) {
		t.Errorf("unknown result err = %v, want ErrBadQuery", err)
	}
	if _, _, err := d.ListPlayerMatches(ctx, with(base, func(q *MatchQuery) { q.Queue = 9 })); !errors.Is(err, ErrBadQuery) {
		t.Errorf("unknown queue err = %v, want ErrBadQuery", err)
	}

	// Queues: ranked is a match mode of the normal game mode and must not be
	// counted as normal; Street Brawl is its own game mode.
	setModes := func(id uint64, game, match int) {
		t.Helper()
		if _, err := d.Pool.Exec(ctx, `UPDATE matches SET game_mode = $2, match_mode = $3 WHERE match_id = $1`, id, game, match); err != nil {
			t.Fatal(err)
		}
	}
	setModes(match+1, 1, 4) // ranked
	setModes(match+2, 4, 1) // street brawl
	defer func() { setModes(match+1, 1, 1); setModes(match+2, 1, 1) }()
	for queue, want := range map[v1.MatchQueue][]uint64{
		v1.MatchQueue_MATCH_QUEUE_UNSPECIFIED:  {match + 1, match, match + 2},
		v1.MatchQueue_MATCH_QUEUE_NORMAL:       {match},
		v1.MatchQueue_MATCH_QUEUE_RANKED:       {match + 1},
		v1.MatchQueue_MATCH_QUEUE_STREET_BRAWL: {match + 2},
		v1.MatchQueue_MATCH_QUEUE_OTHER:        {},
	} {
		rows, total, err := d.ListPlayerMatches(ctx, with(base, func(q *MatchQuery) { q.Queue = queue }))
		if err != nil {
			t.Fatal(err)
		}
		got := []uint64{}
		for _, r := range rows {
			got = append(got, r.MatchId)
		}
		if !slices.Equal(got, want) || total != len(want) {
			t.Errorf("queue %v = %v (total %d), want %v", queue, got, total, want)
		}
	}
	setModes(match+1, 1, 7) // hero labs
	if rows, _, _ := d.ListPlayerMatches(ctx, with(base, func(q *MatchQuery) { q.Queue = v1.MatchQueue_MATCH_QUEUE_OTHER })); len(rows) != 1 || rows[0].MatchId != match+1 {
		t.Errorf("other queue = %v, want the hero labs match", rows)
	}
}

func with(q MatchQuery, f func(*MatchQuery)) MatchQuery {
	f(&q)
	return q
}

// testPeers copies the metadata match into five more: +10 won (teams
// swapped), +11..+13 as is, one ally replaced by stranger 998 in +12 and +13,
// and +14 with all allies replaced. So allies a1..a4 share 5 matches
// (friends), a5 shares 3, 998 shares 2; +14 is the only match without friends.
func testPeers(ctx context.Context, t *testing.T, d *DB, acc uint32, match uint64) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := d.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []int{10, 11, 12, 13, 14} {
		exec(`INSERT INTO matches (match_id, start_time, duration_s, game_mode, match_mode, winning_team)
			SELECT match_id + $2, start_time + make_interval(hours => $2::int), duration_s, game_mode, match_mode, winning_team
			FROM matches WHERE match_id = $1`, match, k)
		exec(`INSERT INTO match_players
			SELECT match_id + $2, slot, account_id, team, hero_id, lane, kills, deaths, assists, net_worth,
			       last_hits, denies, level, player_damage, player_healing, abandon_time_s
			FROM match_players WHERE match_id = $1`, match, k)
	}
	exec(`UPDATE match_players SET team = 1 - team WHERE match_id = $1`, match+10)

	var team int16
	if err := d.Pool.QueryRow(ctx, `SELECT team FROM match_players WHERE match_id = $1 AND account_id = $2`, match, acc).Scan(&team); err != nil {
		t.Fatal(err)
	}
	var allies []uint64
	rows, err := d.Pool.Query(ctx, `SELECT account_id FROM match_players WHERE match_id = $1 AND team = $2 AND account_id <> $3 ORDER BY slot`, match, team, acc)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a uint64
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		allies = append(allies, a)
	}
	if len(allies) != 5 {
		t.Fatalf("allies in the fixture = %v", allies)
	}
	a5 := allies[4]
	exec(`UPDATE match_players SET account_id = 998 WHERE match_id IN ($1, $2) AND account_id = $3`, match+12, match+13, a5)
	exec(`UPDATE match_players SET account_id = account_id + 1000000000 WHERE match_id = $1 AND team = $2 AND account_id <> $3`, match+14, team, acc)

	p, err := d.ListPlayerPeers(ctx, acc)
	if err != nil {
		t.Fatal(err)
	}
	if p.Matches != 6 || p.Wins != 1 {
		t.Errorf("matches, wins = %d, %d; want 6, 1", p.Matches, p.Wins)
	}
	byID := map[uint64]*v1.Peer{}
	for _, a := range p.Allies {
		byID[a.AccountId] = a
	}
	if len(p.Allies) != 6 {
		t.Errorf("allies = %v, want a1..a5 and 998", p.Allies)
	}
	for _, a := range allies[:4] {
		if r := byID[a]; r == nil || r.Matches != 5 || r.Wins != 1 || r.LastMatchId != match+13 || r.TopHeroId == 0 || r.FirstPlayed >= r.LastPlayed {
			t.Errorf("ally %d = %+v", a, r)
		}
	}
	if r := byID[a5]; r == nil || r.Matches != 3 {
		t.Errorf("a5 = %+v, want 3 matches", r)
	}
	if r := byID[998]; r == nil || r.Matches != 2 || r.Wins != 0 {
		t.Errorf("998 = %+v, want 2 matches", r)
	}
	if p.Allies[0].Matches != 5 || p.Allies[len(p.Allies)-1].Matches != 2 {
		t.Errorf("allies not sorted by matches: %v", p.Allies)
	}
	if len(p.Enemies) != 6 {
		t.Fatalf("enemies = %v", p.Enemies)
	}
	for _, e := range p.Enemies {
		if e.Matches != 6 || e.Wins != 1 {
			t.Errorf("enemy = %+v, want 6 matches, 1 win", e)
		}
	}
	var with, against, withWins uint32
	for _, h := range p.Heroes {
		with += h.WithMatches
		against += h.AgainstMatches
		withWins += h.WithWins
	}
	if with != 30 || against != 36 || withWins != 5 {
		t.Errorf("hero matchups with %d (%d wins), against %d; want 30 (5), 36", with, withWins, against)
	}
	want := []*v1.PartySize{{Friends: 0, Matches: 1, Wins: 0}, {Friends: 4, Matches: 5, Wins: 1}}
	if len(p.PartySizes) != len(want) {
		t.Fatalf("party sizes = %v, want %v", p.PartySizes, want)
	}
	for i, w := range want {
		if g := p.PartySizes[i]; g.Friends != w.Friends || g.Matches != w.Matches || g.Wins != w.Wins {
			t.Errorf("party size %d = %+v, want %+v", i, g, w)
		}
	}

	if p, err := d.ListPlayerPeers(ctx, 42); err != nil || p.Matches != 0 || p.Allies != nil || p.PartySizes != nil {
		t.Errorf("unknown player peers = %+v, %v", p, err)
	}
}

// testProfiles: stale accounts come tracked first; stored profiles are
// listed, missing ones remembered; friends of tracked players mark peers.
func testProfiles(ctx context.Context, t *testing.T, d *DB, acc uint32) {
	t.Helper()
	ids, err := d.StaleProfiles(ctx, 7*24*time.Hour, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) < 12 || ids[0] != acc || slices.Contains(ids, 0) {
		t.Fatalf("stale = %d ids starting %v; want the tracked player first and everyone from matches", len(ids), ids[:min(3, len(ids))])
	}
	p, err := d.ListPlayerPeers(ctx, acc)
	if err != nil || len(p.Allies) == 0 {
		t.Fatalf("peers: %v, %v", p, err)
	}
	ally := uint32(p.Allies[0].AccountId)
	if p.Allies[0].SteamFriend {
		t.Error("steam friend before any friend list")
	}

	since := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	got := []source.SteamProfile{
		{AccountID: acc, PersonaName: "me", AvatarMedium: "https://a/me.jpg", Friends: []source.SteamFriend{{AccountID: ally, FriendSince: &since}}},
		{AccountID: ally, PersonaName: "mate", Friends: []source.SteamFriend{{AccountID: 5}}}, // untracked: friends not kept
	}
	now := time.Now()
	if err := d.StoreProfiles(ctx, ids, got, now); err != nil {
		t.Fatal(err)
	}
	if err := d.StoreProfiles(ctx, ids, got, now); err != nil { // idempotent
		t.Fatal(err)
	}
	if left, err := d.StaleProfiles(ctx, 7*24*time.Hour, 1000); err != nil || len(left) != 0 {
		t.Errorf("stale after store = %v, %v; want none (missing ones are remembered)", left, err)
	}
	if left, _ := d.StaleProfiles(ctx, 0, 1000); len(left) != len(ids) {
		t.Errorf("stale with max age 0 = %d, want all %d", len(left), len(ids))
	}

	ps, err := d.ListProfiles(ctx, []uint32{ally, acc, 424242})
	if err != nil || len(ps) != 2 {
		t.Fatalf("profiles = %v, %v; want the two found", ps, err)
	}
	byID := map[uint32]string{}
	for _, x := range ps {
		byID[x.AccountId] = x.PersonaName
	}
	if byID[acc] != "me" || byID[ally] != "mate" {
		t.Errorf("profiles = %v", byID)
	}
	var friends int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM steam_friends`).Scan(&friends); err != nil || friends != 1 {
		t.Errorf("stored friends = %d, %v; want only the tracked player's", friends, err)
	}

	// A profile that disappeared keeps its last name.
	if err := d.StoreProfiles(ctx, []uint32{ally}, nil, now); err != nil {
		t.Fatal(err)
	}
	if ps, _ := d.ListProfiles(ctx, []uint32{ally}); len(ps) != 1 || ps[0].PersonaName != "mate" {
		t.Errorf("vanished profile = %v, want the last known name", ps)
	}

	p, _ = d.ListPlayerPeers(ctx, acc)
	for _, a := range p.Allies {
		if a.SteamFriend != (uint32(a.AccountId) == ally) {
			t.Errorf("ally %d steam friend = %v", a.AccountId, a.SteamFriend)
		}
	}
}

// testDeferMeta: a deferred pending match waits without losing an attempt;
// a stored match is left alone.
func testDeferMeta(ctx context.Context, t *testing.T, d *DB, match uint64) {
	t.Helper()
	pending := match + 1 // history only, still pending
	until := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := d.DeferMeta(ctx, pending, until, errors.New("rate limited")); err != nil {
		t.Fatal(err)
	}
	var (
		status, msg string
		attempts    int
		next        time.Time
	)
	if err := d.Pool.QueryRow(ctx, `SELECT meta_status, meta_error, meta_attempts, meta_next_try FROM matches WHERE match_id = $1`, pending).
		Scan(&status, &msg, &attempts, &next); err != nil {
		t.Fatal(err)
	}
	if status != MetaPending || msg != "rate limited" || attempts != 0 || !next.Equal(until) {
		t.Errorf("deferred = %s %q attempts %d next %v; want pending, no attempt, next %v", status, msg, attempts, next, until)
	}
	if ids, _ := d.NextPendingMeta(ctx, 10); slices.Contains(ids, pending) {
		t.Errorf("deferred match is due: %v", ids)
	}
	if err := d.DeferMeta(ctx, match, until, errors.New("x")); err != nil {
		t.Fatal(err)
	}
	if err := d.Pool.QueryRow(ctx, `SELECT meta_status, meta_error FROM matches WHERE match_id = $1`, match).Scan(&status, &msg); err != nil {
		t.Fatal(err)
	}
	if status != MetaOK || msg != "" {
		t.Errorf("stored match after DeferMeta = %s %q, want untouched", status, msg)
	}
}
