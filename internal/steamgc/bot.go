package steamgc

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/source"
)

// Pacing of the bot: one GC request at a time, at least MinGap apart, and
// reconnects backing off from ReconnectFirst up to ReconnectLongest.
const (
	MinGap           = 3 * time.Second
	ReconnectFirst   = time.Minute
	ReconnectLongest = 30 * time.Minute
	dialTimeout      = time.Minute
	// requestTimeout bounds one GC request: the GC drops some silently.
	requestTimeout = 30 * time.Second
	// readyWait is how long a request waits for a session (startup,
	// reconnects) before the caller falls back to another source.
	readyWait = 30 * time.Second
	readyPoll = 500 * time.Millisecond
)

// Bot keeps a GC session alive and serves source.HistoryProvider and
// source.SaltProvider over it. Without a session its methods fail fast with
// ErrNotReady, so a source.SaltChain / source.MergedHistory moves on.
type Bot struct {
	log      *slog.Logger
	username string
	token    string // a secret: never logged

	mu         sync.Mutex
	cur        *Client
	connecting bool // a dial is in progress

	req  sync.Mutex // one GC request at a time
	last time.Time  // when the previous one was sent
}

// NewBot returns a bot; Run connects it.
func NewBot(log *slog.Logger, username, token string) *Bot {
	return &Bot{log: log, username: username, token: token}
}

// Run keeps the session up until ctx ends.
func (b *Bot) Run(ctx context.Context) {
	backoff := ReconnectFirst
	for ctx.Err() == nil {
		b.setConnecting(true)
		dctx, cancel := context.WithTimeout(ctx, dialTimeout)
		c, err := Dial(dctx, b.log, b.username, b.token)
		cancel()
		b.setConnecting(false)
		if err != nil {
			b.log.Warn("steam gc unavailable", "err", err, "retry_in", backoff)
			if !sleep(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, ReconnectLongest)
			continue
		}
		b.set(c)
		up := time.Now()
		select {
		case <-ctx.Done():
			c.Close()
			b.set(nil)
			return
		case <-c.Done():
			b.set(nil)
			b.log.Warn("steam gc session ended", "err", c.Err())
		}
		// A session that held for a while resets the backoff.
		if time.Since(up) > ReconnectLongest {
			backoff = ReconnectFirst
		}
		if !sleep(ctx, backoff) {
			return
		}
		backoff = min(backoff*2, ReconnectLongest)
	}
}

func (b *Bot) live() *Client {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cur == nil || b.cur.Err() != nil {
		return nil
	}
	return b.cur
}

func (b *Bot) setConnecting(v bool) {
	b.mu.Lock()
	b.connecting = v
	b.mu.Unlock()
}

func (b *Bot) isConnecting() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.connecting
}

func (b *Bot) set(c *Client) {
	b.mu.Lock()
	b.cur = c
	b.mu.Unlock()
}

// session waits for its turn and returns the live session, giving a bot
// that is dialing right now up to readyWait.
func (b *Bot) session(ctx context.Context) (*Client, func(), error) {
	c := b.live()
	for deadline := time.Now().Add(readyWait); c == nil && b.isConnecting() && time.Now().Before(deadline); c = b.live() {
		if !sleep(ctx, readyPoll) {
			return nil, nil, ctx.Err()
		}
	}
	if c == nil {
		return nil, nil, ErrNotReady
	}
	b.req.Lock()
	if wait := MinGap - time.Since(b.last); wait > 0 && !sleep(ctx, wait) {
		b.req.Unlock()
		return nil, nil, ctx.Err()
	}
	return c, func() { b.last = time.Now(); b.req.Unlock() }, nil
}

// MatchHistory is the newest page of a player's history over the live session.
func (b *Bot) MatchHistory(ctx context.Context, accountID uint32) ([]source.MatchSummary, error) {
	c, done, err := b.session(ctx)
	if err != nil {
		return nil, fmt.Errorf("steam gc: match history of %d: %w", accountID, err)
	}
	defer done()
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return c.MatchHistory(ctx, accountID)
}

// Salts are a match's replay salts over the live session.
func (b *Bot) Salts(ctx context.Context, matchID uint64) (source.Salts, error) {
	c, done, err := b.session(ctx)
	if err != nil {
		return source.Salts{}, fmt.Errorf("steam gc: salts of %d: %w", matchID, err)
	}
	defer done()
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return c.Salts(ctx, matchID)
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
