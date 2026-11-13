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
	NextPendingMeta(ctx context.Context, limit int) ([]uint64, error)
	StoreMeta(ctx context.Context, matchID uint64, raw []byte, version uint32, mi *pb.CMsgMatchMetaDataContents_MatchInfo) error
	MarkMetaFailure(ctx context.Context, matchID uint64, expired bool, cause error, maxAttempts int) error

	// Re-derivation of stored matches after the derivation changed.
	NextStaleMeta(ctx context.Context, after uint64, limit int) ([]uint64, error)
	MetaRaw(ctx context.Context, matchID uint64) ([]byte, error)
	RederiveMeta(ctx context.Context, matchID uint64, mi *pb.CMsgMatchMetaDataContents_MatchInfo) error
}

// Syncer coordinates providers and the database.
type Syncer struct {
	DB      Store
	History source.HistoryProvider
	Salts   source.SaltProvider
	HTTP    *http.Client
	Log     *slog.Logger

	// Delay between metadata downloads; keeps us polite to the salt
	// provider and Valve's CDN.
	Interval    time.Duration
	MaxAttempts int

	wake chan struct{}
}

// New returns a Syncer with production defaults; tune the exported fields
// before calling Run.
func New(d Store, hist source.HistoryProvider, salts source.SaltProvider, log *slog.Logger) *Syncer {
	return &Syncer{
		DB: d, History: hist, Salts: salts, Log: log,
		HTTP:        &http.Client{Timeout: 2 * time.Minute},
		Interval:    1500 * time.Millisecond,
		MaxAttempts: 5,
		wake:        make(chan struct{}, 1),
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
		ids, err := s.DB.NextPendingMeta(ctx, 50)
		if err != nil && ctx.Err() == nil {
			s.Log.Error("meta queue", "err", err)
		}
		for _, id := range ids {
			if ctx.Err() != nil {
				return
			}
			s.processMeta(ctx, id)
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.Interval):
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

// errExpired marks replays Valve no longer hosts.
var errExpired = errors.New("replay expired")

func (s *Syncer) processMeta(ctx context.Context, matchID uint64) {
	raw, version, mi, err := s.fetchMeta(ctx, matchID)
	if err == nil {
		err = s.DB.StoreMeta(ctx, matchID, raw, version, mi)
	}
	if err == nil {
		s.Log.Info("meta stored", "match", matchID, "players", len(mi.GetPlayers()))
		return
	}
	if ctx.Err() != nil {
		return
	}
	expired := errors.Is(err, errExpired)
	s.Log.Warn("meta failed", "match", matchID, "expired", expired, "err", err)
	if e := s.DB.MarkMetaFailure(ctx, matchID, expired, err, s.MaxAttempts); e != nil {
		s.Log.Error("mark meta failure", "match", matchID, "err", e)
	}
}

func (s *Syncer) fetchMeta(ctx context.Context, matchID uint64) ([]byte, uint32, *pb.CMsgMatchMetaDataContents_MatchInfo, error) {
	salts, err := s.Salts.Salts(ctx, matchID)
	if err != nil {
		if errors.Is(err, source.ErrNotFound) {
			return nil, 0, nil, fmt.Errorf("%w: %w", errExpired, err)
		}
		return nil, 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, salts.MetadataURL(), nil)
	if err != nil {
		return nil, 0, nil, err
	}
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, 0, nil, fmt.Errorf("%w: %s", errExpired, salts.MetadataURL())
	case resp.StatusCode != http.StatusOK:
		return nil, 0, nil, fmt.Errorf("metadata %s: %s", salts.MetadataURL(), resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
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
