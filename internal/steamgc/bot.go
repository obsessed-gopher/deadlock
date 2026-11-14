package steamgc

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/source"
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

	// A process runs a bot only while it holds the bot's lease: LeaseTTL
	// long, renewed every LeaseRenew. A process that dies frees the bot
	// within LeaseTTL.
	LeaseTTL   = 2 * time.Minute
	LeaseRenew = 30 * time.Second
	// storeTimeout bounds the budget and lease writes that must happen even
	// when the request that caused them is gone.
	storeTimeout = 5 * time.Second
)

// kind is a budget of one sort of GC request, stored under name.
type kind struct {
	name string
	limits
}

var (
	saltsKind   = kind{"salts", limits{SaltsPerHour, saltsBurst, saltsReserve}}
	historyKind = kind{"history", limits{HistoryPerHour, historyBurst, 0}}
)

// Account is a bot's login: the name and the refresh token from Login (a
// secret: never logged).
type Account struct {
	Username string
	Token    string
}

// Bot keeps one GC session alive while its process holds the bot's lease;
// the request budget lives in the Store.
type Bot struct {
	log   *slog.Logger
	acc   Account
	store Store
	owner string // this process in the lease table
	dial  func(ctx context.Context) (*Client, error)

	leaseTTL, leaseRenew time.Duration

	mu         sync.Mutex
	cur        *Client
	connecting bool
	last       time.Time // when the previous request was sent

	req sync.Mutex // one GC request at a time
}

// NewBot returns a bot of the owner process; Run connects it.
func NewBot(log *slog.Logger, acc Account, store Store, owner string) *Bot {
	b := &Bot{
		log: log.With("bot", acc.Username), acc: acc, store: store, owner: owner,
		leaseTTL: LeaseTTL, leaseRenew: LeaseRenew,
	}
	b.dial = func(ctx context.Context) (*Client, error) { return Dial(ctx, b.log, acc.Username, acc.Token) }
	return b
}

// Run keeps the session up until ctx ends, reconnecting with a backoff. The
// session only runs while this process holds the bot's lease; otherwise Run
// waits for the lease.
func (b *Bot) Run(ctx context.Context) {
	defer func() {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
		defer cancel()
		if err := b.store.ReleaseBot(rctx, b.acc.Username, b.owner); err != nil {
			b.log.Warn("steam bot lease not released", "err", err)
		}
	}()
	backoff := ReconnectFirst
	for ctx.Err() == nil {
		if !b.acquire(ctx) {
			return
		}
		b.setConnecting(true)
		dctx, cancel := context.WithTimeout(ctx, dialTimeout)
		c, err := b.dial(dctx)
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
		b.hold(ctx, c)
		b.set(nil)
		if ctx.Err() != nil {
			return
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

// acquire waits until this process holds the bot's lease; false if ctx ended.
func (b *Bot) acquire(ctx context.Context) bool {
	waiting := false
	for {
		ok, err := b.store.LeaseBot(ctx, b.acc.Username, b.owner, b.leaseTTL)
		switch {
		case ctx.Err() != nil:
			return false
		case err != nil:
			b.log.Warn("steam bot lease", "err", err)
		case ok:
			if waiting {
				b.log.Info("steam bot taken over from another process")
			}
			return true
		case !waiting:
			b.log.Info("steam bot runs in another process, waiting for its lease")
			waiting = true
		}
		if !sleep(ctx, b.leaseRenew) {
			return false
		}
	}
}

// hold renews the lease while the session lives. It closes the session when
// ctx ends or the lease is lost (taken by another process, or not renewed
// for so long that it may have been), and returns once the session is over.
func (b *Bot) hold(ctx context.Context, c *Client) {
	t := time.NewTicker(b.leaseRenew)
	defer t.Stop()
	renewed := time.Now()
	for {
		select {
		case <-ctx.Done():
			c.Close()
			return
		case <-c.Done():
			b.log.Warn("steam gc session ended", "err", c.Err())
			return
		case <-t.C:
		}
		ok, err := b.store.LeaseBot(ctx, b.acc.Username, b.owner, b.leaseTTL)
		switch {
		case ctx.Err() != nil:
			continue // ctx.Done closes the session
		case ok:
			renewed = time.Now()
			continue
		case err == nil:
			b.log.Warn("steam bot lease taken by another process, logging off")
		case time.Since(renewed) < b.leaseTTL-b.leaseRenew:
			b.log.Warn("steam bot lease not renewed, retrying", "err", err)
			continue
		default:
			b.log.Warn("steam bot lease lost, logging off", "err", err)
		}
		c.Close()
		return
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

// reserve spends a token of the kind's stored budget or tells how long to
// wait.
func (b *Bot) reserve(ctx context.Context, k kind, fresh bool) (wait time.Duration, ok bool, err error) {
	now := time.Now()
	err = b.store.UpdateBotBudget(ctx, b.acc.Username, k.name, k.full(now), func(s *source.BotBudget) {
		wait, ok = k.take(s, now, fresh)
	})
	return wait, ok, err
}

// outcome feeds a GC answer back into the budget: a rate limit pauses the
// kind with an escalating backoff, a success resets it. It is recorded even
// if the request's ctx is gone.
func (b *Bot) outcome(ctx context.Context, k kind, err error) {
	if err != nil && !isRateLimit(err) {
		return
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
	defer cancel()
	now := time.Now()
	var pause time.Duration
	var strikes int
	serr := b.store.UpdateBotBudget(sctx, b.acc.Username, k.name, k.full(now), func(s *source.BotBudget) {
		if err != nil {
			pause = limited(s, now)
			strikes = s.Strikes
		} else {
			succeeded(s)
		}
	})
	if serr != nil {
		b.log.Warn("steam gc budget not saved", "kind", k.name, "err", serr)
	}
	if err != nil {
		b.log.Warn("steam gc rate limit", "kind", k.name, "pause", pause.String(), "in_a_row", strikes)
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
