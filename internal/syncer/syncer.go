// Package syncer keeps the database up to date: it pulls players' match
// history and downloads + ingests match metadata in the background.
package syncer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/compress"
	"github.com/obsessed-gopher/deadlock/internal/meta"
	"github.com/obsessed-gopher/deadlock/internal/pb"
	"github.com/obsessed-gopher/deadlock/internal/source"
)

// Store is the write side of the database the syncer needs. *db.DB implements it.
type Store interface {
	EnsurePlayer(ctx context.Context, accountID uint32) error
	SetSyncState(ctx context.Context, accountID uint32, state, errMsg string, synced bool) error
	UpsertHistory(ctx context.Context, accountID uint32, ms []source.MatchSummary) (newMatches int, err error)
	CountPendingMeta(ctx context.Context, accountID uint32) (int, error)
	ClaimPendingMeta(ctx context.Context, limit int, lease time.Duration) ([]source.QueuedMatch, error)
	StoreMeta(ctx context.Context, matchID uint64, raw []byte, version uint32, mi *pb.CMsgMatchMetaDataContents_MatchInfo) error
	MarkMetaFailure(ctx context.Context, matchID uint64, expired bool, cause error, maxAttempts int) error
	DeferMeta(ctx context.Context, matchID uint64, until time.Time, cause error) error

	// Re-derivation of stored matches after the derivation changed.
	NextStaleMeta(ctx context.Context, after uint64, limit int) ([]uint64, error)
	MetaRaw(ctx context.Context, matchID uint64) ([]byte, error)
	RederiveMeta(ctx context.Context, matchID uint64, mi *pb.CMsgMatchMetaDataContents_MatchInfo) error

	// Steam profiles of everyone seen.
	StaleProfiles(ctx context.Context, maxAge time.Duration, limit int) ([]uint32, error)
	StoreProfiles(ctx context.Context, requested []uint32, got []source.SteamProfile, at time.Time) error
}

// The worker takes metaBatch queued matches at a time and leases them for
// metaLease: enough to download a batch, short enough that matches of a
// worker that died come back soon.
const (
	metaBatch = 10
	metaLease = 10 * time.Minute
)

// Syncer coordinates providers and the database.
type Syncer struct {
	DB      Store
	History source.HistoryProvider
	Salts   source.SaltProvider
	// Meta, when set, is asked for the .meta file first; salts and Valve's
	// CDN are the fallback for matches it doesn't have.
	Meta source.MetaProvider
	HTTP *http.Client
	Log  *slog.Logger

	// Delay between metadata downloads; keeps us polite to the salt
	// provider and Valve's CDN.
	Interval    time.Duration
	MaxAttempts int

	// Steam profiles (names, avatars); nil Profiles disables RunProfiles.
	// One batch per ProfileEvery while there is work, a check per
	// ProfileIdle otherwise; a profile is refreshed after ProfileMaxAge.
	Profiles      source.ProfileProvider
	ProfileEvery  time.Duration
	ProfileIdle   time.Duration
	ProfileMaxAge time.Duration

	wake chan struct{}

	// The salts provider is out of budget: no salts requests of a priority
	// before this moment. Touched only by the Run goroutine.
	saltsPausedUntil map[source.Priority]time.Time
}

// New returns a Syncer with production defaults; tune the exported fields
// before calling Run.
func New(d Store, hist source.HistoryProvider, salts source.SaltProvider, log *slog.Logger) *Syncer {
	return &Syncer{
		DB: d, History: hist, Salts: salts, Log: log,
		HTTP:          &http.Client{Timeout: 2 * time.Minute},
		Interval:      1500 * time.Millisecond,
		MaxAttempts:   5,
		ProfileEvery:  time.Minute,
		ProfileIdle:   15 * time.Minute,
		ProfileMaxAge: 7 * 24 * time.Hour,
		wake:          make(chan struct{}, 1),

		saltsPausedUntil: map[source.Priority]time.Time{},
	}
}

// SyncPlayer pulls the player's match history and queues new matches.
func (s *Syncer) SyncPlayer(ctx context.Context, accountID uint32) (newMatches, pending int, err error) {
	if err := s.DB.EnsurePlayer(ctx, accountID); err != nil {
		return 0, 0, err
	}
	if err := s.DB.SetSyncState(ctx, accountID, "running", "", false); err != nil {
		return 0, 0, err
	}
	defer func() {
		state, msg := "idle", ""
		if err != nil {
			state, msg = "error", err.Error()
		}
		if e := s.DB.SetSyncState(context.WithoutCancel(ctx), accountID, state, msg, err == nil); e != nil {
			s.Log.Error("set sync state", "account", accountID, "err", e)
		}
	}()

	ms, err := s.History.MatchHistory(ctx, accountID)
	if err != nil {
		return 0, 0, err
	}
	newMatches, err = s.DB.UpsertHistory(ctx, accountID, ms)
	if err != nil {
		return 0, 0, err
	}
	pending, err = s.DB.CountPendingMeta(ctx, accountID)
	if err != nil {
		return newMatches, 0, err
	}
	s.Log.Info("player synced", "account", accountID, "history", len(ms), "new", newMatches, "pending_meta", pending)
	s.Wake()
	return newMatches, pending, nil
}

// Wake nudges the metadata worker to check the queue now.
func (s *Syncer) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Rederive rebuilds the derived tables of every stored match whose
// derivation is out of date, from meta_raw alone: no network. A match that
// fails is logged and skipped, so one bad file never blocks the rest.
func (s *Syncer) Rederive(ctx context.Context) (done, failed int, err error) {
	var after uint64
	for {
		ids, err := s.DB.NextStaleMeta(ctx, after, 100)
		if err != nil || len(ids) == 0 {
			return done, failed, err
		}
		for _, id := range ids {
			if err := s.rederiveOne(ctx, id); err != nil {
				if ctx.Err() != nil {
					return done, failed, ctx.Err()
				}
				failed++
				s.Log.Warn("rederive failed", "match", id, "err", err)
				continue
			}
			done++
		}
		after = ids[len(ids)-1]
	}
}

func (s *Syncer) rederiveOne(ctx context.Context, matchID uint64) error {
	raw, err := s.DB.MetaRaw(ctx, matchID)
	if err != nil {
		return err
	}
	plain, err := compress.ReadAll(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("decompress: %w", err)
	}
	mi, err := meta.Decode(plain)
	if err != nil {
		return err
	}
	return s.DB.RederiveMeta(ctx, matchID, mi)
}

// Run brings stale derived tables up to date, then processes the metadata
// queue until ctx is cancelled.
func (s *Syncer) Run(ctx context.Context) {
	if done, failed, err := s.Rederive(ctx); err != nil && ctx.Err() == nil {
		s.Log.Error("rederive", "err", err)
	} else if done+failed > 0 {
		s.Log.Info("rederived stored matches", "done", done, "failed", failed)
	}
	idle := time.NewTicker(time.Minute)
	defer idle.Stop()
	for {
		// Small batches: a match that arrives meanwhile (a fresh one, newest
		// first) waits for at most this many downloads.
		ids, err := s.DB.ClaimPendingMeta(ctx, metaBatch, metaLease)
		if err != nil && ctx.Err() == nil {
			s.Log.Error("meta queue", "err", err)
		}
		for _, q := range ids {
			if ctx.Err() != nil {
				return
			}
			// Old matches get only the spare budget of the salts provider.
			pctx := source.WithPriority(ctx, source.PriorityFor(q.StartTime, time.Now()))
			pause := s.processMeta(pctx, q.MatchID)
			select {
			case <-ctx.Done():
				return
			case <-time.After(max(s.Interval, pause)):
			}
		}
		if len(ids) > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-idle.C:
		}
	}
}

// RefreshProfiles fetches one batch of missing or outdated Steam profiles
// and returns how many accounts it asked for.
func (s *Syncer) RefreshProfiles(ctx context.Context) (int, error) {
	ids, err := s.DB.StaleProfiles(ctx, s.ProfileMaxAge, source.MaxProfileBatch)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	got, err := s.Profiles.SteamProfiles(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("steam profiles: %w", err)
	}
	return len(ids), s.DB.StoreProfiles(ctx, ids, got, time.Now())
}

// RunProfiles keeps Steam profiles fresh until ctx is cancelled. A full
// batch means more work is waiting: the next one follows after ProfileEvery.
// Otherwise (or after an error) it rests for ProfileIdle.
func (s *Syncer) RunProfiles(ctx context.Context) {
	if s.Profiles == nil {
		return
	}
	for {
		n, err := s.RefreshProfiles(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			s.Log.Warn("steam profiles", "err", err)
		case n > 0:
			s.Log.Info("steam profiles refreshed", "accounts", n)
		}
		wait := s.ProfileIdle
		if err == nil && n == source.MaxProfileBatch {
			wait = s.ProfileEvery
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// pauseSalts holds back salts requests of a priority until at least until.
func (s *Syncer) pauseSalts(p source.Priority, until time.Time) {
	if s.saltsPausedUntil == nil {
		s.saltsPausedUntil = map[source.Priority]time.Time{}
	}
	if until.After(s.saltsPausedUntil[p]) {
		s.saltsPausedUntil[p] = until
	}
}

// errExpired marks replays Valve no longer hosts.
var errExpired = errors.New("replay expired")

// deferredError postpones a match without counting an attempt; pause also
// holds back the whole queue (the provider of every download is limited).
type deferredError struct {
	until time.Time
	pause time.Duration
	err   error
}

func (e *deferredError) Error() string { return e.err.Error() }
func (e *deferredError) Unwrap() error { return e.err }

// processMeta downloads and stores one match. It returns how long the queue
// must wait before the next download beyond the usual Interval.
func (s *Syncer) processMeta(ctx context.Context, matchID uint64) time.Duration {
	raw, version, mi, err := s.fetchMeta(ctx, matchID)
	if err == nil {
		err = s.DB.StoreMeta(ctx, matchID, raw, version, mi)
	}
	if err == nil {
		s.Log.Info("meta stored", "match", matchID, "players", len(mi.GetPlayers()))
		return 0
	}
	if ctx.Err() != nil {
		return 0
	}
	if d, ok := errors.AsType[*deferredError](err); ok {
		s.Log.Info("meta deferred", "match", matchID, "until", d.until.Format(time.RFC3339), "err", err)
		if e := s.DB.DeferMeta(ctx, matchID, d.until, err); e != nil {
			s.Log.Error("defer meta", "match", matchID, "err", e)
		}
		return d.pause
	}
	expired := errors.Is(err, errExpired)
	s.Log.Warn("meta failed", "match", matchID, "expired", expired, "err", err)
	if e := s.DB.MarkMetaFailure(ctx, matchID, expired, err, s.MaxAttempts); e != nil {
		s.Log.Error("mark meta failure", "match", matchID, "err", e)
	}
	return 0
}

func (s *Syncer) fetchMeta(ctx context.Context, matchID uint64) ([]byte, uint32, *pb.CMsgMatchMetaDataContents_MatchInfo, error) {
	raw, err := s.downloadMeta(ctx, matchID)
	if err != nil {
		return nil, 0, nil, err
	}
	plain, err := compress.ReadAll(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("decompress: %w", err)
	}
	version, mi, err := meta.DecodeWithVersion(plain)
	if err != nil {
		return nil, 0, nil, err
	}
	return raw, version, mi, nil
}

// downloadMeta returns the compressed .meta file: from s.Meta when it has
// it, otherwise via salts from Valve's CDN.
func (s *Syncer) downloadMeta(ctx context.Context, matchID uint64) ([]byte, error) {
	if s.Meta != nil {
		raw, err := s.Meta.MetadataRaw(ctx, matchID)
		if rl, ok := errors.AsType[*source.RateLimitError](err); ok {
			return nil, &deferredError{until: time.Now().Add(rl.RetryAfter), pause: rl.RetryAfter, err: err}
		}
		if !errors.Is(err, source.ErrNotFound) {
			return raw, err
		}
	}
	prio := source.PriorityOf(ctx)
	if until := s.saltsPausedUntil[prio]; time.Now().Before(until) {
		return nil, &deferredError{until: until, err: errors.New("salts: paused after a rate limit")}
	}
	salts, err := s.Salts.Salts(ctx, matchID)
	if rl, ok := errors.AsType[*source.RateLimitError](err); ok {
		// Out of budget (the GC bots', deadlock-api's 3 an hour from Steam):
		// stop asking until it refills; matches deadlock-api stores are still
		// downloaded meanwhile. Fresh matches out of budget hold back the
		// backlog too; the backlog out of budget leaves fresh matches be.
		until := time.Now().Add(rl.RetryAfter)
		s.pauseSalts(prio, until)
		if prio == source.Fresh {
			s.pauseSalts(source.Backlog, until)
		}
		return nil, &deferredError{until: until, err: err}
	}
	if err != nil {
		if errors.Is(err, source.ErrNotFound) {
			return nil, fmt.Errorf("%w: %w", errExpired, err)
		}
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, salts.MetadataURL(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s", errExpired, salts.MetadataURL())
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("metadata %s: %s", salts.MetadataURL(), resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, source.MaxMetaSize))
}
