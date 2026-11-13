// Package server implements deadlock.v1.StatsService.
package server

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/db"
	"github.com/obsessed-gopher/deadlock/internal/gamedata"
	"github.com/obsessed-gopher/deadlock/internal/source"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/obsessed-gopher/deadlock/gen/go/deadlock/v1"
)

// Store is the read side of the database the API needs. *db.DB implements it.
type Store interface {
	GetPlayer(ctx context.Context, accountID uint32) (*v1.Player, error)
	ListPlayerMatches(ctx context.Context, q db.MatchQuery) ([]*v1.PlayerMatchRow, int, error)
	ListPlayerHeroStats(ctx context.Context, accountID uint32) ([]*v1.HeroStats, error)
	ListPlayerMatchFacts(ctx context.Context, accountID uint32) ([]*v1.MatchFacts, error)
	GetMatch(ctx context.Context, matchID uint64) (*v1.Match, error)
	GetPlayerMatch(ctx context.Context, matchID uint64, accountID uint32) (*v1.PlayerMatch, error)
}

// PlayerSyncer triggers a history sync. *syncer.Syncer implements it.
type PlayerSyncer interface {
	SyncPlayer(ctx context.Context, accountID uint32) (newMatches, pending int, err error)
}

// Server implements v1.StatsServiceServer.
type Server struct {
	v1.UnimplementedStatsServiceServer
	store  Store
	syncer PlayerSyncer

	gameData *v1.GameData
}

// New builds the service; the game data snapshot is converted once here.
func New(store Store, sy PlayerSyncer, gd *gamedata.Data) *Server {
	return &Server{store: store, syncer: sy, gameData: gameDataToAPI(gd)}
}

// GetPlayer implements v1.StatsServiceServer.
func (s *Server) GetPlayer(ctx context.Context, req *v1.GetPlayerRequest) (*v1.Player, error) {
	id, err := accountID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	p, err := s.store.GetPlayer(ctx, id)
	return p, toStatus(err)
}

// ListPlayerMatches implements v1.StatsServiceServer.
func (s *Server) ListPlayerMatches(ctx context.Context, req *v1.ListPlayerMatchesRequest) (*v1.ListPlayerMatchesResponse, error) {
	id, err := accountID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	size := int(req.GetPageSize())
	switch {
	case size <= 0:
		size = 50
	case size > 200:
		size = 200
	}
	if _, ok := v1.MatchSort_name[int32(req.GetSort())]; !ok {
		return nil, status.Error(codes.InvalidArgument, "unknown sort")
	}
	if _, ok := v1.MatchResult_name[int32(req.GetResult())]; !ok {
		return nil, status.Error(codes.InvalidArgument, "unknown result")
	}
	if _, ok := v1.MetaStatus_name[int32(req.GetMetaStatus())]; !ok {
		return nil, status.Error(codes.InvalidArgument, "unknown meta_status")
	}
	q := db.MatchQuery{
		AccountID: id, HeroID: req.GetHeroId(), Result: req.GetResult(),
		GameMode: req.GetGameMode(), MetaStatus: req.GetMetaStatus(),
		Sort: req.GetSort(), Ascending: req.GetAscending(), Limit: size,
	}
	if t := req.GetPageToken(); t != "" {
		c, err := parsePageToken(t)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "bad page_token")
		}
		q.After = c
	}
	rows, total, err := s.store.ListPlayerMatches(ctx, q)
	if err != nil {
		return nil, toStatus(err)
	}
	resp := &v1.ListPlayerMatchesResponse{Matches: rows, Total: uint32(total)}
	if len(rows) == size {
		last := rows[len(rows)-1]
		resp.NextPageToken = formatPageToken(db.MatchSortValue(last, q.Sort), last.GetMatchId())
	}
	return resp, nil
}

// Page tokens are "<sort value>_<match id>" of the last row. Clients treat
// them as opaque; a token is only valid with the sort it was issued for.
func formatPageToken(value int64, matchID uint64) string {
	return strconv.FormatInt(value, 10) + "_" + strconv.FormatUint(matchID, 10)
}

func parsePageToken(t string) (*db.MatchCursor, error) {
	v, m, ok := strings.Cut(t, "_")
	if !ok {
		return nil, errors.New("no separator")
	}
	value, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, err
	}
	matchID, err := strconv.ParseUint(m, 10, 64)
	if err != nil {
		return nil, err
	}
	return &db.MatchCursor{Value: value, MatchID: matchID}, nil
}

// ListPlayerHeroStats implements v1.StatsServiceServer.
func (s *Server) ListPlayerHeroStats(ctx context.Context, req *v1.ListPlayerHeroStatsRequest) (*v1.ListPlayerHeroStatsResponse, error) {
	id, err := accountID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	heroes, err := s.store.ListPlayerHeroStats(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &v1.ListPlayerHeroStatsResponse{Heroes: heroes}, nil
}

// ListPlayerMatchFacts implements v1.StatsServiceServer.
func (s *Server) ListPlayerMatchFacts(ctx context.Context, req *v1.ListPlayerMatchFactsRequest) (*v1.ListPlayerMatchFactsResponse, error) {
	id, err := accountID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	facts, err := s.store.ListPlayerMatchFacts(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return &v1.ListPlayerMatchFactsResponse{
		Matches:     facts,
		SoulMarksS:  db.SoulMarksS,
		SoulTargets: db.SoulTargets,
	}, nil
}

// SyncPlayer implements v1.StatsServiceServer.
func (s *Server) SyncPlayer(ctx context.Context, req *v1.SyncPlayerRequest) (*v1.SyncPlayerResponse, error) {
	id, err := accountID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	n, pending, err := s.syncer.SyncPlayer(ctx, id)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "sync: %v", err)
	}
	return &v1.SyncPlayerResponse{NewMatches: uint32(n), QueuedMeta: uint32(pending)}, nil
}

// GetMatch implements v1.StatsServiceServer.
func (s *Server) GetMatch(ctx context.Context, req *v1.GetMatchRequest) (*v1.Match, error) {
	if req.GetMatchId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "match_id is required")
	}
	m, err := s.store.GetMatch(ctx, req.GetMatchId())
	return m, toStatus(err)
}

// GetPlayerMatch implements v1.StatsServiceServer.
func (s *Server) GetPlayerMatch(ctx context.Context, req *v1.GetPlayerMatchRequest) (*v1.PlayerMatch, error) {
	if req.GetMatchId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "match_id is required")
	}
	id, err := accountID(req.GetAccountId())
	if err != nil {
		return nil, err
	}
	pm, err := s.store.GetPlayerMatch(ctx, req.GetMatchId(), id)
	return pm, toStatus(err)
}

// GetGameData implements v1.StatsServiceServer.
func (s *Server) GetGameData(_ context.Context, _ *v1.GetGameDataRequest) (*v1.GameData, error) {
	return s.gameData, nil
}

// gameDataToAPI converts the embedded snapshot once at startup.
func gameDataToAPI(d *gamedata.Data) *v1.GameData {
	out := &v1.GameData{GameVersion: d.GameVersion}
	for _, h := range d.Heroes {
		out.Heroes = append(out.Heroes, &v1.Hero{
			Id: h.ID, Key: h.Key, Name: h.Name, NameRu: h.NameRU, Type: h.Type,
			Playable: h.State == "release" && !h.Disabled && !h.InDev,
		})
	}
	for _, a := range d.Abilities {
		out.Abilities = append(out.Abilities, &v1.Ability{
			Id: a.ID, Key: a.Key, Name: a.Name, NameRu: a.NameRU, Type: a.Type,
			Slot: a.Slot, Tier: uint32(a.Tier),
		})
	}
	return out
}

// accountID validates a request account id, which may also be a SteamID64.
func accountID(v uint64) (uint32, error) {
	if v == 0 {
		return 0, status.Error(codes.InvalidArgument, "account_id is required")
	}
	id, ok := source.NormalizeAccountID(v)
	if !ok {
		return 0, status.Error(codes.InvalidArgument, "account_id is neither an account id nor a SteamID64")
	}
	return id, nil
}

func toStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, db.ErrNotFound):
		return status.Error(codes.NotFound, "not found")
	case errors.Is(err, db.ErrBadQuery):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// UnaryInterceptor logs every call, converts panics into Internal errors and
// hides the text of internal errors from clients.
func UnaryInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		start := time.Now()
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic", "method", info.FullMethod, "panic", r, "stack", string(debug.Stack()))
				err = status.Error(codes.Internal, "internal error")
			}
			code := status.Code(err)
			level := slog.LevelInfo
			if code == codes.Internal || code == codes.Unknown {
				level = slog.LevelError
			}
			log.Log(ctx, level, "rpc", "method", info.FullMethod, "code", code.String(),
				"dur_ms", time.Since(start).Milliseconds(), "err", err)
			if level == slog.LevelError {
				// The details are in the log; clients must not see SQL or internals.
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}
