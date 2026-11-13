package source

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type saltsFunc func() (Salts, error)

func (f saltsFunc) Salts(context.Context, uint64) (Salts, error) { return f() }

type historyFunc func() ([]MatchSummary, error)

func (f historyFunc) MatchHistory(context.Context, uint32) ([]MatchSummary, error) { return f() }

func TestSaltChain(t *testing.T) {
	ok := saltsFunc(func() (Salts, error) { return Salts{MatchID: 1, ClusterID: 7}, nil })
	missing := saltsFunc(func() (Salts, error) { return Salts{}, ErrNotFound })
	down := saltsFunc(func() (Salts, error) { return Salts{}, errors.New("no session") })
	limited := saltsFunc(func() (Salts, error) { return Salts{}, &RateLimitError{What: "x", RetryAfter: time.Hour} })

	t.Run("first success wins", func(t *testing.T) {
		called := false
		second := saltsFunc(func() (Salts, error) { called = true; return Salts{}, nil })
		s, err := SaltChain{ok, second}.Salts(context.Background(), 1)
		if err != nil || s.ClusterID != 7 || called {
			t.Errorf("got %+v, %v, second called=%v", s, err, called)
		}
	})
	t.Run("falls through to the next", func(t *testing.T) {
		s, err := SaltChain{down, ok}.Salts(context.Background(), 1)
		if err != nil || s.ClusterID != 7 {
			t.Errorf("got %+v, %v", s, err)
		}
	})
	cases := []struct {
		name         string
		chain        SaltChain
		notFound, rl bool
	}{
		{"all missing: the match is gone", SaltChain{missing, missing}, true, false},
		{"one down, one missing: not gone, try again", SaltChain{down, missing}, false, false},
		{"rate limited: try later", SaltChain{missing, limited}, false, true},
		{"rate limit beats a plain failure", SaltChain{down, limited}, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.chain.Salts(context.Background(), 1)
			if err == nil {
				t.Fatal("want an error")
			}
			if got := errors.Is(err, ErrNotFound); got != c.notFound {
				t.Errorf("ErrNotFound = %v, want %v (%v)", got, c.notFound, err)
			}
			if _, got := errors.AsType[*RateLimitError](err); got != c.rl {
				t.Errorf("RateLimitError = %v, want %v (%v)", got, c.rl, err)
			}
		})
	}
}

func TestMergedHistory(t *testing.T) {
	gc := historyFunc(func() ([]MatchSummary, error) {
		return []MatchSummary{{MatchID: 30, StartTime: 300, Kills: 9}, {MatchID: 20, StartTime: 200, Kills: 9}}, nil
	})
	api := historyFunc(func() ([]MatchSummary, error) {
		return []MatchSummary{{MatchID: 20, StartTime: 200, Kills: 1}, {MatchID: 10, StartTime: 100}}, nil
	})
	down := historyFunc(func() ([]MatchSummary, error) { return nil, errors.New("down") })

	got, err := MergedHistory{Providers: []HistoryProvider{gc, api}}.MatchHistory(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []MatchSummary{{MatchID: 30, StartTime: 300, Kills: 9}, {MatchID: 20, StartTime: 200, Kills: 9}, {MatchID: 10, StartTime: 100}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged = %+v, want %+v (the first provider's entry wins, newest first)", got, want)
	}

	var failed []int
	h := MergedHistory{Providers: []HistoryProvider{down, api}, OnError: func(i int, _ error) { failed = append(failed, i) }}
	got, err = h.MatchHistory(context.Background(), 1)
	if err != nil || len(got) != 2 || !reflect.DeepEqual(failed, []int{0}) {
		t.Errorf("one down: got %d matches, err %v, failed %v", len(got), err, failed)
	}

	if _, err := (MergedHistory{Providers: []HistoryProvider{down, down}}).MatchHistory(context.Background(), 1); err == nil {
		t.Error("all down: want an error")
	}
}
