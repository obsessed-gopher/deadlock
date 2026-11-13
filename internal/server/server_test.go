package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/obsessed-gopher/deadlock/internal/db"
	"github.com/obsessed-gopher/deadlock/internal/gamedata"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v1 "github.com/obsessed-gopher/deadlock/gen/go/deadlock/v1"
)

// fakeStore records the arguments of the last call and returns canned data.
type fakeStore struct {
	Store // panics on anything a test did not expect

	player *v1.Player
	rows   []*v1.PlayerMatchRow
	heroes []*v1.HeroStats
	err    error

	gotAccount uint32
	gotQuery   db.MatchQuery
	total      int
}

func (f *fakeStore) GetPlayer(_ context.Context, accountID uint32) (*v1.Player, error) {
	f.gotAccount = accountID
	return f.player, f.err
}

func (f *fakeStore) ListPlayerMatches(_ context.Context, q db.MatchQuery) ([]*v1.PlayerMatchRow, int, error) {
	f.gotAccount, f.gotQuery = q.AccountID, q
	return f.rows, f.total, f.err
}

func (f *fakeStore) ListPlayerHeroStats(_ context.Context, accountID uint32) ([]*v1.HeroStats, error) {
	f.gotAccount = accountID
	return f.heroes, f.err
}

func (f *fakeStore) ListPlayerMatchFacts(_ context.Context, accountID uint32) ([]*v1.MatchFacts, error) {
	f.gotAccount = accountID
	return nil, f.err
}

func (f *fakeStore) GetPlayerMatch(_ context.Context, _ uint64, accountID uint32) (*v1.PlayerMatch, error) {
	f.gotAccount = accountID
	return &v1.PlayerMatch{}, f.err
}

type fakeSyncer struct {
	newMatches, pending int
	err                 error
}

func (f fakeSyncer) SyncPlayer(context.Context, uint32) (int, int, error) {
	return f.newMatches, f.pending, f.err
}

// recordingSyncer remembers the account it was asked to sync.
type recordingSyncer struct{ got uint32 }

func (r *recordingSyncer) SyncPlayer(_ context.Context, accountID uint32) (int, int, error) {
	r.got = accountID
	return 0, 0, nil
}

func newServer(st Store, sy PlayerSyncer) *Server {
	return New(st, sy, &gamedata.Data{})
}

func TestRequiredIDs(t *testing.T) {
	s := newServer(&fakeStore{}, fakeSyncer{})
	ctx := context.Background()
	calls := map[string]func() error{
		"GetPlayer":         func() error { _, err := s.GetPlayer(ctx, &v1.GetPlayerRequest{}); return err },
		"ListPlayerMatches": func() error { _, err := s.ListPlayerMatches(ctx, &v1.ListPlayerMatchesRequest{}); return err },
		"SyncPlayer":        func() error { _, err := s.SyncPlayer(ctx, &v1.SyncPlayerRequest{}); return err },
		"ListPlayerHeroStats": func() error {
			_, err := s.ListPlayerHeroStats(ctx, &v1.ListPlayerHeroStatsRequest{})
			return err
		},
		"ListPlayerMatchFacts": func() error {
			_, err := s.ListPlayerMatchFacts(ctx, &v1.ListPlayerMatchFactsRequest{})
			return err
		},
		"GetMatch": func() error { _, err := s.GetMatch(ctx, &v1.GetMatchRequest{}); return err },
		"GetPlayerMatch": func() error {
			_, err := s.GetPlayerMatch(ctx, &v1.GetPlayerMatchRequest{MatchId: 1})
			return err
		},
	}
	for name, call := range calls {
		if got := status.Code(call()); got != codes.InvalidArgument {
			t.Errorf("%s with missing id: code = %s, want InvalidArgument", name, got)
		}
	}
}

func TestAccountIDAcceptsSteamID64(t *testing.T) {
	const (
		steamID64 = 76561199477771562
		account   = 1517505834
	)
	st, sy := &fakeStore{}, &recordingSyncer{}
	s := newServer(st, sy)
	ctx := context.Background()
	calls := map[string]func(id uint64) (uint32, error){
		"GetPlayer": func(id uint64) (uint32, error) {
			_, err := s.GetPlayer(ctx, &v1.GetPlayerRequest{AccountId: id})
			return st.gotAccount, err
		},
		"ListPlayerMatches": func(id uint64) (uint32, error) {
			_, err := s.ListPlayerMatches(ctx, &v1.ListPlayerMatchesRequest{AccountId: id})
			return st.gotAccount, err
		},
		"GetPlayerMatch": func(id uint64) (uint32, error) {
			_, err := s.GetPlayerMatch(ctx, &v1.GetPlayerMatchRequest{MatchId: 1, AccountId: id})
			return st.gotAccount, err
		},
		"ListPlayerHeroStats": func(id uint64) (uint32, error) {
			_, err := s.ListPlayerHeroStats(ctx, &v1.ListPlayerHeroStatsRequest{AccountId: id})
			return st.gotAccount, err
		},
		"ListPlayerMatchFacts": func(id uint64) (uint32, error) {
			_, err := s.ListPlayerMatchFacts(ctx, &v1.ListPlayerMatchFactsRequest{AccountId: id})
			return st.gotAccount, err
		},
		"SyncPlayer": func(id uint64) (uint32, error) {
			_, err := s.SyncPlayer(ctx, &v1.SyncPlayerRequest{AccountId: id})
			return sy.got, err
		},
	}
	for name, call := range calls {
		for _, id := range []uint64{account, steamID64} {
			st.gotAccount, sy.got = 0, 0
			got, err := call(id)
			if err != nil || got != account {
				t.Errorf("%s(%d): store got %d, err %v; want %d", name, id, got, err, account)
			}
		}
		st.gotAccount, sy.got = 0, 0
		if _, err := call(4_294_967_296); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s(2^32): code = %s, want InvalidArgument", name, status.Code(err))
		}
	}
}

func TestGetPlayerErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		want codes.Code
	}{
		{nil, codes.OK},
		{db.ErrNotFound, codes.NotFound},
		{context.Canceled, codes.Canceled},
		{context.DeadlineExceeded, codes.DeadlineExceeded},
		{errors.New("connection refused"), codes.Internal},
	}
	for _, c := range cases {
		s := newServer(&fakeStore{player: &v1.Player{AccountId: 1}, err: c.err}, fakeSyncer{})
		_, err := s.GetPlayer(context.Background(), &v1.GetPlayerRequest{AccountId: 1})
		if got := status.Code(err); got != c.want {
			t.Errorf("store error %v: code = %s, want %s", c.err, got, c.want)
		}
	}
}

func TestListPlayerMatchesPaging(t *testing.T) {
	cases := []struct {
		name      string
		req       *v1.ListPlayerMatchesRequest
		rows      int
		wantLimit int
		wantToken string
	}{
		{"default size", &v1.ListPlayerMatchesRequest{AccountId: 1}, 3, 50, ""},
		{"size is capped", &v1.ListPlayerMatchesRequest{AccountId: 1, PageSize: 10_000}, 3, 200, ""},
		// Token = sort value of the last row + its match id; default sort is start time.
		{"full page has a next token", &v1.ListPlayerMatchesRequest{AccountId: 1, PageSize: 3}, 3, 3, "1001_101"},
		{
			"token carries the value of the requested sort",
			&v1.ListPlayerMatchesRequest{AccountId: 1, PageSize: 3, Sort: v1.MatchSort_MATCH_SORT_KILLS}, 3, 3, "7_101",
		},
		{"short page ends the list", &v1.ListPlayerMatchesRequest{AccountId: 1, PageSize: 4}, 3, 4, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := &fakeStore{total: 42}
			for i := range c.rows {
				id := uint64(103 - i)
				st.rows = append(st.rows, &v1.PlayerMatchRow{MatchId: id, StartTime: int64(id) + 900, Kills: uint32(id) - 94})
			}
			resp, err := newServer(st, fakeSyncer{}).ListPlayerMatches(context.Background(), c.req)
			if err != nil {
				t.Fatal(err)
			}
			if st.gotQuery.Limit != c.wantLimit {
				t.Errorf("limit = %d, want %d", st.gotQuery.Limit, c.wantLimit)
			}
			if resp.GetNextPageToken() != c.wantToken {
				t.Errorf("next token = %q, want %q", resp.GetNextPageToken(), c.wantToken)
			}
			if resp.GetTotal() != 42 {
				t.Errorf("total = %d, want 42", resp.GetTotal())
			}
		})
	}
}

func TestListPlayerMatchesQuery(t *testing.T) {
	st := &fakeStore{}
	s := newServer(st, fakeSyncer{})
	_, err := s.ListPlayerMatches(context.Background(), &v1.ListPlayerMatchesRequest{
		AccountId: 1, HeroId: 13, PageToken: "-5_101", Sort: v1.MatchSort_MATCH_SORT_NET_WORTH, Ascending: true,
		Result: v1.MatchResult_MATCH_RESULT_WIN, GameMode: 4, MetaStatus: v1.MetaStatus_META_STATUS_OK,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := db.MatchQuery{
		AccountID: 1, HeroID: 13, Result: v1.MatchResult_MATCH_RESULT_WIN, GameMode: 4,
		MetaStatus: v1.MetaStatus_META_STATUS_OK, Sort: v1.MatchSort_MATCH_SORT_NET_WORTH, Ascending: true,
		After: &db.MatchCursor{Value: -5, MatchID: 101}, Limit: 50,
	}
	got := st.gotQuery
	if got.After == nil || *got.After != *want.After {
		t.Fatalf("cursor = %+v, want %+v", got.After, want.After)
	}
	got.After, want.After = nil, nil
	if got != want {
		t.Errorf("query = %+v, want %+v", got, want)
	}
}

func TestListPlayerMatchesRejectsBadInput(t *testing.T) {
	cases := map[string]*v1.ListPlayerMatchesRequest{
		"garbage token":       {AccountId: 1, PageToken: "garbage"},
		"token without id":    {AccountId: 1, PageToken: "5_"},
		"old token format":    {AccountId: 1, PageToken: "101"},
		"unknown sort":        {AccountId: 1, Sort: 99},
		"unknown result":      {AccountId: 1, Result: 7},
		"unknown meta status": {AccountId: 1, MetaStatus: 9},
	}
	for name, req := range cases {
		st := &fakeStore{}
		_, err := newServer(st, fakeSyncer{}).ListPlayerMatches(context.Background(), req)
		if got := status.Code(err); got != codes.InvalidArgument {
			t.Errorf("%s: code = %s, want InvalidArgument", name, got)
		}
	}
	// A query the store rejects is the client's fault, not an internal error.
	st := &fakeStore{err: fmt.Errorf("%w: sort", db.ErrBadQuery)}
	_, err := newServer(st, fakeSyncer{}).ListPlayerMatches(context.Background(), &v1.ListPlayerMatchesRequest{AccountId: 1})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("store ErrBadQuery: code = %s, want InvalidArgument", got)
	}
}

func TestListPlayerHeroStats(t *testing.T) {
	heroes := []*v1.HeroStats{{HeroId: 13, Matches: 2}, {HeroId: 7, Matches: 1}}
	s := newServer(&fakeStore{heroes: heroes}, fakeSyncer{})
	resp, err := s.ListPlayerHeroStats(context.Background(), &v1.ListPlayerHeroStatsRequest{AccountId: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetHeroes()) != 2 || resp.GetHeroes()[0].GetHeroId() != 13 {
		t.Errorf("heroes = %v, want the store rows in order", resp.GetHeroes())
	}

	s = newServer(&fakeStore{err: errors.New("connection refused")}, fakeSyncer{})
	_, err = s.ListPlayerHeroStats(context.Background(), &v1.ListPlayerHeroStatsRequest{AccountId: 1})
	if got := status.Code(err); got != codes.Internal {
		t.Errorf("store failure: code = %s, want Internal", got)
	}
}

func TestSyncPlayer(t *testing.T) {
	s := newServer(&fakeStore{}, fakeSyncer{newMatches: 4, pending: 9})
	resp, err := s.SyncPlayer(context.Background(), &v1.SyncPlayerRequest{AccountId: 1})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetNewMatches() != 4 || resp.GetQueuedMeta() != 9 {
		t.Errorf("resp = %v", resp)
	}

	s = newServer(&fakeStore{}, fakeSyncer{err: errors.New("provider down")})
	_, err = s.SyncPlayer(context.Background(), &v1.SyncPlayerRequest{AccountId: 1})
	if got := status.Code(err); got != codes.Unavailable {
		t.Errorf("provider failure: code = %s, want Unavailable", got)
	}
}

func TestGameDataToAPI(t *testing.T) {
	gd := &gamedata.Data{
		GameVersion: "6000",
		Heroes: []*gamedata.Hero{
			{ID: 13, Key: "hero_haze", Name: "Haze", NameRU: "Пелена", State: "release"},
			{ID: 99, Key: "hero_wip", State: "release", InDev: true},
			{ID: 98, Key: "hero_off", State: "release", Disabled: true},
			{ID: 97, Key: "hero_test", State: "playtest"},
		},
		Abilities: []*gamedata.Ability{{ID: 7, Key: "upgrade_x", Type: "item", Slot: "weapon", Tier: 2}},
	}
	out, err := New(nil, nil, gd).GetGameData(context.Background(), &v1.GetGameDataRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if out.GetGameVersion() != "6000" || len(out.GetHeroes()) != 4 || len(out.GetAbilities()) != 1 {
		t.Fatalf("out = %v", out)
	}
	for i, want := range []bool{true, false, false, false} {
		if got := out.GetHeroes()[i].GetPlayable(); got != want {
			t.Errorf("hero %s playable = %v, want %v", out.GetHeroes()[i].GetKey(), got, want)
		}
	}
	if a := out.GetAbilities()[0]; a.GetSlot() != "weapon" || a.GetTier() != 2 {
		t.Errorf("ability = %v", a)
	}
}

func TestInterceptor(t *testing.T) {
	icpt := UnaryInterceptor(slog.New(slog.DiscardHandler))
	info := &grpc.UnaryServerInfo{FullMethod: "/deadlock.v1.StatsService/GetPlayer"}
	call := func(h grpc.UnaryHandler) error {
		_, err := icpt(context.Background(), nil, info, h)
		return err
	}

	err := call(func(context.Context, any) (any, error) { panic("boom") })
	if status.Code(err) != codes.Internal {
		t.Errorf("panic: err = %v, want Internal", err)
	}

	err = call(func(context.Context, any) (any, error) {
		return nil, status.Error(codes.Internal, `ERROR: relation "matches" does not exist`)
	})
	if st := status.Convert(err); st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Errorf("internal error leaked to the client: %v", err)
	}

	err = call(func(context.Context, any) (any, error) {
		return nil, status.Error(codes.NotFound, "not found")
	})
	if st := status.Convert(err); st.Code() != codes.NotFound || st.Message() != "not found" {
		t.Errorf("client error must pass through unchanged, got %v", err)
	}

	if err := call(func(context.Context, any) (any, error) { return "ok", nil }); err != nil {
		t.Errorf("success: err = %v", err)
	}
}
