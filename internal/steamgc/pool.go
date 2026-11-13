package steamgc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/source"
)

// ErrNotFriend means no bot of the pool has the account as a Steam friend:
// the GC shows match history only to friends.
var ErrNotFriend = errors.New("steam gc: account is not a friend of any bot")

// How long a request waits for a bot that is dialing right now (startup,
// reconnects) before the caller falls back to another source.
const (
	readyWait = 30 * time.Second
	readyPoll = 500 * time.Millisecond
)

// Pool serves source.HistoryProvider and source.SaltProvider over several
// bots: the GC budget is per account, so more bots mean more requests an
// hour. Salts go to any bot with budget left; history goes to a bot that has
// the account as a friend (bots accept friend requests on their own).
type Pool struct {
	bots []*Bot
}

// NewPool makes a bot per account; Run connects them.
func NewPool(log *slog.Logger, accounts []Account) *Pool {
	p := &Pool{}
	for _, a := range accounts {
		p.bots = append(p.bots, NewBot(log, a))
	}
	return p
}

// Run keeps every bot connected until ctx ends.
func (p *Pool) Run(ctx context.Context) {
	done := make(chan struct{})
	for _, b := range p.bots {
		go func() { b.Run(ctx); done <- struct{}{} }()
	}
	for range p.bots {
		<-done
	}
}

// live waits (briefly, only while some bot is dialing) for live sessions.
func (p *Pool) live(ctx context.Context) ([]*Bot, []*Client, error) {
	deadline := time.Now().Add(readyWait)
	for {
		var bots []*Bot
		var cs []*Client
		dialing := false
		for _, b := range p.bots {
			c, d := b.state()
			dialing = dialing || d
			if c != nil {
				bots = append(bots, b)
				cs = append(cs, c)
			}
		}
		if len(cs) > 0 || !dialing || time.Now().After(deadline) {
			if len(cs) == 0 {
				return nil, nil, ErrNotReady
			}
			return bots, cs, nil
		}
		if !sleep(ctx, readyPoll) {
			return nil, nil, ctx.Err()
		}
	}
}

// Salts asks a bot with budget left; when none has any, the error is a
// *source.RateLimitError with the shortest wait.
func (p *Pool) Salts(ctx context.Context, matchID uint64) (source.Salts, error) {
	bots, cs, err := p.live(ctx)
	if err != nil {
		return source.Salts{}, fmt.Errorf("steam gc: salts of %d: %w", matchID, err)
	}
	fresh := source.PriorityOf(ctx) == source.Fresh
	var wait time.Duration
	for i, b := range bots {
		w, ok := b.reserve(b.salts, fresh)
		if !ok {
			if wait == 0 || w < wait {
				wait = w
			}
			continue
		}
		release, err := b.turn(ctx)
		if err != nil {
			return source.Salts{}, err
		}
		rctx, cancel := context.WithTimeout(ctx, requestTimeout)
		s, err := cs[i].Salts(rctx, matchID)
		cancel()
		release()
		b.outcome(b.salts, err)
		return s, err
	}
	return source.Salts{}, &source.RateLimitError{What: "steam gc salts budget", RetryAfter: max(wait, time.Second)}
}

// MatchHistory asks a bot that has the account as a friend.
func (p *Pool) MatchHistory(ctx context.Context, accountID uint32) ([]source.MatchSummary, error) {
	bots, cs, err := p.live(ctx)
	if err != nil {
		return nil, fmt.Errorf("steam gc: match history of %d: %w", accountID, err)
	}
	var wait time.Duration
	friend := false
	for i, b := range bots {
		if !cs[i].IsFriend(accountID) {
			continue
		}
		friend = true
		w, ok := b.reserve(b.history, true)
		if !ok {
			if wait == 0 || w < wait {
				wait = w
			}
			continue
		}
		release, err := b.turn(ctx)
		if err != nil {
			return nil, err
		}
		rctx, cancel := context.WithTimeout(ctx, requestTimeout)
		ms, err := cs[i].MatchHistory(rctx, accountID)
		cancel()
		release()
		b.outcome(b.history, err)
		return ms, err
	}
	if !friend {
		return nil, fmt.Errorf("match history of %d: %w", accountID, ErrNotFriend)
	}
	return nil, &source.RateLimitError{What: "steam gc history budget", RetryAfter: max(wait, time.Second)}
}

func isRateLimit(err error) bool {
	_, ok := errors.AsType[*source.RateLimitError](err)
	return ok
}
