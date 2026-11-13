package syncer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/pb"
	"github.com/obsessed-gopher/deadlock/internal/source"
)

const (
	testMatch   = 134969900
	testAccount = 1517505834
	testMeta    = "../meta/testdata/134969900.meta.zst"
)

// fakeStore is an in-memory Store that records what the syncer did.
type fakeStore struct {
	Store // panics on anything a test did not expect

	historyErr error
	newMatches int
	pending    int

	states []string // sync states in the order they were set
	synced bool

	storedMatch   uint64
	storedRaw     []byte
	storedVersion uint32
	storedPlayers int

	failedMatch   uint64
	failedExpired bool
	failedCause   error

	staleProfiles  []uint32
	gotMaxAge      time.Duration
	storedAsked    []uint32
	storedProfiles []source.SteamProfile

	deferredMatch uint64
	deferredUntil time.Time
	deferredCause error
}

func (f *fakeStore) StaleProfiles(_ context.Context, maxAge time.Duration, limit int) ([]uint32, error) {
	f.gotMaxAge = maxAge
	return f.staleProfiles[:min(limit, len(f.staleProfiles))], nil
}

func (f *fakeStore) StoreProfiles(_ context.Context, requested []uint32, got []source.SteamProfile, _ time.Time) error {
	f.storedAsked, f.storedProfiles = requested, got
	return nil
}

// fakeProfiles knows profiles of even account ids.
type fakeProfiles struct {
	calls int
	err   error
}

func (p *fakeProfiles) SteamProfiles(_ context.Context, ids []uint32) ([]source.SteamProfile, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	var out []source.SteamProfile
	for _, id := range ids {
		if id%2 == 0 {
			out = append(out, source.SteamProfile{AccountID: id, PersonaName: "p"})
		}
	}
	return out, nil
}

func TestRefreshProfiles(t *testing.T) {
	st := &fakeStore{}
	prov := &fakeProfiles{}
	s := newSyncer(st, nil, nil, nil)
	s.Profiles = prov
	ctx := context.Background()

	// Nothing stale: no request at all.
	if n, err := s.RefreshProfiles(ctx); err != nil || n != 0 || prov.calls != 0 {
		t.Fatalf("idle refresh = %d, %v, calls %d", n, err, prov.calls)
	}

	for i := range source.MaxProfileBatch + 5 {
		st.staleProfiles = append(st.staleProfiles, uint32(i+1))
	}
	n, err := s.RefreshProfiles(ctx)
	if err != nil || n != source.MaxProfileBatch || prov.calls != 1 {
		t.Fatalf("refresh = %d, %v, calls %d; want one full batch", n, err, prov.calls)
	}
	if st.gotMaxAge != s.ProfileMaxAge || len(st.storedAsked) != source.MaxProfileBatch || len(st.storedProfiles) != source.MaxProfileBatch/2 {
		t.Errorf("stored %d asked, %d found (max age %v)", len(st.storedAsked), len(st.storedProfiles), st.gotMaxAge)
	}

	// A provider failure stores nothing: the accounts stay stale and are
	// retried after ProfileIdle.
	st.storedAsked = nil
	prov.err = errors.New("503")
	if _, err := s.RefreshProfiles(ctx); err == nil || st.storedAsked != nil {
		t.Errorf("provider failure: err %v, stored %v", err, st.storedAsked)
	}
}

func (f *fakeStore) EnsurePlayer(context.Context, uint32) error { return nil }

func (f *fakeStore) SetSyncState(_ context.Context, _ uint32, state, _ string, synced bool) error {
	f.states = append(f.states, state)
	f.synced = synced
	return nil
}

func (f *fakeStore) UpsertHistory(context.Context, uint32, []source.MatchSummary) (int, error) {
	return f.newMatches, f.historyErr
}

func (f *fakeStore) CountPendingMeta(context.Context, uint32) (int, error) { return f.pending, nil }

func (f *fakeStore) StoreMeta(_ context.Context, matchID uint64, raw []byte, version uint32, mi *pb.CMsgMatchMetaDataContents_MatchInfo) error {
	f.storedMatch, f.storedRaw, f.storedVersion, f.storedPlayers = matchID, raw, version, len(mi.GetPlayers())
	return nil
}

func (f *fakeStore) DeferMeta(_ context.Context, matchID uint64, until time.Time, cause error) error {
	f.deferredMatch, f.deferredUntil, f.deferredCause = matchID, until, cause
	return nil
}

func (f *fakeStore) MarkMetaFailure(_ context.Context, matchID uint64, expired bool, cause error, _ int) error {
	f.failedMatch, f.failedExpired, f.failedCause = matchID, expired, cause
	return nil
}

type fakeHistory struct {
	ms  []source.MatchSummary
	err error
}

func (f fakeHistory) MatchHistory(context.Context, uint32) ([]source.MatchSummary, error) {
	return f.ms, f.err
}

type fakeSalts struct{ err error }

func (f fakeSalts) Salts(_ context.Context, matchID uint64) (source.Salts, error) {
	return source.Salts{MatchID: matchID, ClusterID: 1, MetadataSalt: 2}, f.err
}

// countingSalts records how often salts were asked for.
type countingSalts struct {
	calls int
	err   error
}

func (c *countingSalts) Salts(_ context.Context, matchID uint64) (source.Salts, error) {
	c.calls++
	return source.Salts{MatchID: matchID, ClusterID: 1, MetadataSalt: 2}, c.err
}

// fakeMeta serves the .meta file from deadlock-api's storage.
type fakeMeta struct {
	raw []byte
	err error
}

func (f fakeMeta) MetadataRaw(context.Context, uint64) ([]byte, error) { return f.raw, f.err }

// roundTrip serves canned responses instead of Valve's CDN.
type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func respond(code int, body []byte) roundTrip {
	return func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: code,
			Status:     http.StatusText(code),
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     http.Header{},
		}, nil
	}
}

func newSyncer(st Store, hist source.HistoryProvider, salts source.SaltProvider, rt http.RoundTripper) *Syncer {
	s := New(st, hist, salts, slog.New(slog.DiscardHandler))
	s.HTTP = &http.Client{Transport: rt}
	return s
}

func TestSyncPlayer(t *testing.T) {
	st := &fakeStore{newMatches: 2, pending: 5}
	s := newSyncer(st, fakeHistory{ms: make([]source.MatchSummary, 3)}, fakeSalts{}, nil)

	n, pending, err := s.SyncPlayer(context.Background(), testAccount)
	if err != nil || n != 2 || pending != 5 {
		t.Fatalf("SyncPlayer = %d, %d, %v; want 2, 5, nil", n, pending, err)
	}
	if got := st.states; len(got) != 2 || got[0] != "running" || got[1] != "idle" || !st.synced {
		t.Errorf("states = %v synced = %v; want [running idle] true", got, st.synced)
	}
	select {
	case <-s.wake:
	default:
		t.Error("metadata worker was not woken after a sync")
	}
}

func TestSyncPlayerProviderError(t *testing.T) {
	st := &fakeStore{}
	s := newSyncer(st, fakeHistory{err: errors.New("provider down")}, fakeSalts{}, nil)

	if _, _, err := s.SyncPlayer(context.Background(), testAccount); err == nil {
		t.Fatal("want error")
	}
	if got := st.states; len(got) != 2 || got[1] != "error" || st.synced {
		t.Errorf("states = %v synced = %v; want [running error] false", got, st.synced)
	}
}

func TestProcessMetaStores(t *testing.T) {
	raw, err := os.ReadFile(testMeta)
	if err != nil {
		t.Fatal(err)
	}
	st := &fakeStore{}
	s := newSyncer(st, fakeHistory{}, fakeSalts{}, respond(http.StatusOK, raw))

	s.processMeta(context.Background(), testMatch)

	if st.failedMatch != 0 {
		t.Fatalf("unexpected failure: %v", st.failedCause)
	}
	if st.storedMatch != testMatch || st.storedPlayers != 12 {
		t.Errorf("stored match %d with %d players", st.storedMatch, st.storedPlayers)
	}
	// The archive must be kept byte-for-byte: it is our only copy once Valve
	// deletes the replay.
	if !bytes.Equal(st.storedRaw, raw) {
		t.Error("stored bytes differ from the downloaded file")
	}
}

func TestProcessMetaFailures(t *testing.T) {
	cases := []struct {
		name        string
		salts       fakeSalts
		rt          roundTrip
		wantExpired bool
	}{
		{"cdn 404 is permanent", fakeSalts{}, respond(http.StatusNotFound, nil), true},
		{"no salts is permanent", fakeSalts{err: source.ErrNotFound}, nil, true},
		{"cdn 500 is retried", fakeSalts{}, respond(http.StatusBadGateway, nil), false},
		{"salt provider down is retried", fakeSalts{err: errors.New("timeout")}, nil, false},
		{"garbage body is retried", fakeSalts{}, respond(http.StatusOK, []byte{0xff, 0xff, 0xff}), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := &fakeStore{}
			var rt http.RoundTripper
			if c.rt != nil {
				rt = c.rt
			}
			s := newSyncer(st, fakeHistory{}, c.salts, rt)

			s.processMeta(context.Background(), testMatch)

			if st.storedMatch != 0 {
				t.Fatal("metadata must not be stored on failure")
			}
			if st.failedMatch != testMatch || st.failedExpired != c.wantExpired {
				t.Errorf("failure recorded for %d, expired = %v; want %d, %v (cause: %v)",
					st.failedMatch, st.failedExpired, testMatch, c.wantExpired, st.failedCause)
			}
		})
	}
}

func TestProcessMetaPrefersStoredMeta(t *testing.T) {
	raw, err := os.ReadFile(testMeta)
	if err != nil {
		t.Fatal(err)
	}
	st, salts := &fakeStore{}, &countingSalts{}
	cdn := roundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("CDN must not be asked"); return nil, nil })
	s := newSyncer(st, fakeHistory{}, salts, cdn)
	s.Meta = fakeMeta{raw: raw}

	if pause := s.processMeta(context.Background(), testMatch); pause != 0 {
		t.Errorf("pause = %s", pause)
	}
	if st.storedMatch != testMatch || !bytes.Equal(st.storedRaw, raw) || salts.calls != 0 {
		t.Errorf("stored %d (same bytes %v), salts asked %d times; want stored without salts",
			st.storedMatch, bytes.Equal(st.storedRaw, raw), salts.calls)
	}

	// Not in deadlock-api's storage: salts and the CDN, as before.
	st = &fakeStore{}
	s = newSyncer(st, fakeHistory{}, fakeSalts{}, respond(http.StatusOK, raw))
	s.Meta = fakeMeta{err: source.ErrNotFound}
	s.processMeta(context.Background(), testMatch)
	if st.storedMatch != testMatch {
		t.Errorf("fallback: stored %d, failure %v", st.storedMatch, st.failedCause)
	}
}

func TestProcessMetaRateLimits(t *testing.T) {
	ctx := context.Background()

	// 429 from the metadata storage: defer the match, hold the queue, no attempt burnt.
	st := &fakeStore{}
	s := newSyncer(st, fakeHistory{}, &countingSalts{}, nil)
	s.Meta = fakeMeta{err: &source.RateLimitError{What: "meta", RetryAfter: 30 * time.Second}}
	before := time.Now()
	if pause := s.processMeta(ctx, testMatch); pause != 30*time.Second {
		t.Errorf("pause = %s, want 30s", pause)
	}
	if st.failedMatch != 0 || st.deferredMatch != testMatch || st.deferredUntil.Before(before.Add(30*time.Second)) {
		t.Errorf("failed %d, deferred %d until %v", st.failedMatch, st.deferredMatch, st.deferredUntil)
	}

	// 429 from salts: defer, and stop asking for salts until the window ends,
	// while the queue itself keeps going.
	st = &fakeStore{}
	salts := &countingSalts{err: &source.RateLimitError{What: "salts", RetryAfter: 20 * time.Minute}}
	s = newSyncer(st, fakeHistory{}, salts, nil)
	s.Meta = fakeMeta{err: source.ErrNotFound}
	if pause := s.processMeta(ctx, testMatch); pause != 0 {
		t.Errorf("salts 429 pause = %s, want 0: stored matches can still be downloaded", pause)
	}
	if st.failedMatch != 0 || st.deferredMatch != testMatch {
		t.Fatalf("salts 429: failed %d, deferred %d", st.failedMatch, st.deferredMatch)
	}
	until := st.deferredUntil
	st.deferredMatch = 0
	s.processMeta(ctx, testMatch+1)
	if salts.calls != 1 || st.deferredMatch != testMatch+1 || !st.deferredUntil.Equal(until) || st.failedMatch != 0 {
		t.Errorf("while paused: salts asked %d times, deferred %d until %v (want %v)", salts.calls, st.deferredMatch, st.deferredUntil, until)
	}
}

func TestProcessMetaSaltsPausePerPriority(t *testing.T) {
	fresh := context.Background()
	backlog := source.WithPriority(fresh, source.Backlog)
	limited := &source.RateLimitError{What: "salts", RetryAfter: time.Hour}

	// The backlog out of budget: old matches wait, fresh ones still ask.
	st := &fakeStore{}
	salts := &countingSalts{err: limited}
	s := newSyncer(st, fakeHistory{}, salts, nil)
	s.Meta = fakeMeta{err: source.ErrNotFound}
	s.processMeta(backlog, testMatch)
	s.processMeta(backlog, testMatch+1)
	if salts.calls != 1 {
		t.Errorf("backlog while its budget is out: salts asked %d times, want 1", salts.calls)
	}
	s.processMeta(fresh, testMatch+2)
	if salts.calls != 2 {
		t.Errorf("a fresh match behind a paused backlog: salts asked %d times, want 2", salts.calls)
	}

	// Fresh out of budget: everything waits.
	st = &fakeStore{}
	salts = &countingSalts{err: limited}
	s = newSyncer(st, fakeHistory{}, salts, nil)
	s.Meta = fakeMeta{err: source.ErrNotFound}
	s.processMeta(fresh, testMatch)
	s.processMeta(backlog, testMatch+1)
	s.processMeta(fresh, testMatch+2)
	if salts.calls != 1 || st.failedMatch != 0 {
		t.Errorf("after a fresh rate limit: salts asked %d times (want 1), failed %d", salts.calls, st.failedMatch)
	}
}

func TestProcessMetaCancelledIsNotAFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	st := &fakeStore{}
	s := newSyncer(st, fakeHistory{}, fakeSalts{err: context.Canceled}, nil)

	s.processMeta(ctx, testMatch)

	if st.failedMatch != 0 {
		t.Error("shutdown must not burn a retry attempt")
	}
}

// staleStore serves stored .meta files for Rederive and records what was
// re-derived.
type staleStore struct {
	Store

	raws      map[uint64][]byte // match -> meta_raw; derived on rederive
	rederived []uint64
	players   int
}

func (f *staleStore) NextStaleMeta(_ context.Context, after uint64, limit int) ([]uint64, error) {
	var ids []uint64
	for id := range f.raws {
		if id > after && !slices.Contains(f.rederived, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids[:min(limit, len(ids))], nil
}

func (f *staleStore) MetaRaw(_ context.Context, id uint64) ([]byte, error) { return f.raws[id], nil }

func (f *staleStore) RederiveMeta(_ context.Context, id uint64, mi *pb.CMsgMatchMetaDataContents_MatchInfo) error {
	f.rederived = append(f.rederived, id)
	f.players = len(mi.GetPlayers())
	return nil
}

func TestRederive(t *testing.T) {
	raw, err := os.ReadFile(testMeta)
	if err != nil {
		t.Fatal(err)
	}
	// A corrupt file is skipped, not retried forever and not fatal.
	st := &staleStore{raws: map[uint64][]byte{testMatch: raw, 1: []byte("not zstd")}}
	s := New(st, nil, nil, slog.New(slog.DiscardHandler))
	done, failed, err := s.Rederive(context.Background())
	if err != nil || done != 1 || failed != 1 {
		t.Fatalf("Rederive = %d, %d, %v; want 1 done, 1 failed", done, failed, err)
	}
	if !slices.Equal(st.rederived, []uint64{testMatch}) || st.players != 12 {
		t.Errorf("rederived %v with %d players", st.rederived, st.players)
	}
}
