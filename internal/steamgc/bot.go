package steamgc

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Pacing of one bot. The GC answered "rate limited" after ~50 salts in a row
// (the window looked longer than 10 minutes), so salts run at 40 an hour with
// half of the burst kept for fresh matches: the backlog gets one request every
// ~1.5 minutes and a match that just ended never waits for it.
const (
	SaltsPerHour   = 40
	saltsBurst     = 40
	saltsReserve   = 20
	HistoryPerHour = 60
	historyBurst   = 20

	// MinGap is the least time between two requests of one bot.
	MinGap = 3 * time.Second

	ReconnectFirst   = time.Minute
	ReconnectLongest = 30 * time.Minute
	dialTimeout      = time.Minute
	// requestTimeout bounds one GC request: the GC drops some silently.
	requestTimeout = 30 * time.Second
)

// Account is a bot's login: the name and the refresh token from Login (a
// secret: never logged).
type Account struct {
	Username string
	Token    string
}

// Bot keeps one GC session alive and owns that account's request budget.
type Bot struct {
	log *slog.Logger
	acc Account

	mu         sync.Mutex
	cur        *Client
	connecting bool
	salts      *bucket
	history    *bucket
	last       time.Time // when the previous request was sent

	req sync.Mutex // one GC request at a time
}

// NewBot returns a bot; Run connects it.
func NewBot(log *slog.Logger, acc Account) *Bot {
	now := time.Now()
	return &Bot{
		log:     log.With("bot", acc.Username),
		acc:     acc,
		salts:   newBucket(SaltsPerHour, saltsBurst, saltsReserve, now),
		history: newBucket(HistoryPerHour, historyBurst, 0, now),
	}
}

// Run keeps the session up until ctx ends, reconnecting with a backoff.
func (b *Bot) Run(ctx context.Context) {
	backoff := ReconnectFirst
	for ctx.Err() == nil {
		b.setConnecting(true)
		dctx, cancel := context.WithTimeout(ctx, dialTimeout)
		c, err := Dial(dctx, b.log, b.acc.Username, b.acc.Token)
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

func (b *Bot) set(c *Client) {
	b.mu.Lock()
	b.cur = c
	b.mu.Unlock()
}

func (b *Bot) setConnecting(v bool) {
	b.mu.Lock()
	b.connecting = v
	b.mu.Unlock()
}

// state is the live session (nil if none) and whether a dial is under way.
func (b *Bot) state() (*Client, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cur == nil || b.cur.Err() != nil {
		return nil, b.connecting
	}
	return b.cur, b.connecting
}

// reserve spends a token of the kind's budget or tells how long to wait.
func (b *Bot) reserve(kind *bucket, fresh bool) (time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return kind.take(time.Now(), fresh)
}

// outcome feeds a GC answer back into the budget: a rate limit pauses the
// kind with an escalating backoff, a success resets it.
func (b *Bot) outcome(kind *bucket, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if isRateLimit(err) {
		d := kind.limited(time.Now())
		b.log.Warn("steam gc rate limit", "pause", d.String(), "in_a_row", kind.strikes)
		return
	}
	if err == nil {
		kind.succeeded()
	}
}

// turn waits for the bot's previous request to be MinGap old; release must
// be called after the request.
func (b *Bot) turn(ctx context.Context) (release func(), err error) {
	b.req.Lock()
	b.mu.Lock()
	wait := MinGap - time.Since(b.last)
	b.mu.Unlock()
	if wait > 0 && !sleep(ctx, wait) {
		b.req.Unlock()
		return nil, ctx.Err()
	}
	return func() {
		b.mu.Lock()
		b.last = time.Now()
		b.mu.Unlock()
		b.req.Unlock()
	}, nil
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
