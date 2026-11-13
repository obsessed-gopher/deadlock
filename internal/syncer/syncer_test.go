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
