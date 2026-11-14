package steamgc

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/source"
)

// fakeSession is a Client without a connection: Close ends it.
func fakeSession() *Client {
	c := &Client{done: make(chan struct{})}
	var once sync.Once
	c.stop = func() { once.Do(func() { close(c.done) }) }
	return c
}

func testBot(store Store, owner string, dials *atomic.Int32, sessions chan<- *Client) *Bot {
	b := NewBot(slog.New(slog.DiscardHandler), Account{Username: "bot", Token: "t"}, store, owner)
	b.leaseTTL, b.leaseRenew = 200*time.Millisecond, 10*time.Millisecond
	b.dial = func(context.Context) (*Client, error) {
		dials.Add(1)
		c := fakeSession()
		sessions <- c
		return c, nil
	}
	return b
}

func TestBotRunsOnlyWithLease(t *testing.T) {
	ctx := context.Background()
	store := &MemoryStore{}
	if ok, _ := store.LeaseBot(ctx, "bot", "other", time.Hour); !ok {
		t.Fatal("setup: lease refused")
	}
	var dials atomic.Int32
	sessions := make(chan *Client, 4)
	b := testBot(store, "me", &dials, sessions)
	rctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { b.Run(rctx); close(done) }()

	time.Sleep(50 * time.Millisecond)
	if n := dials.Load(); n != 0 {
		t.Fatalf("dialed %d times while another process holds the bot", n)
	}
	// The other process lets go: the bot logs on.
	_ = store.ReleaseBot(ctx, "bot", "other")
	var c *Client
	select {
	case c = <-sessions:
	case <-time.After(time.Second):
		t.Fatal("no dial after the lease was freed")
	}
	// Another process takes the lease over: the bot logs off and waits.
	store.mu.Lock()
	store.leases["bot"] = memLease{owner: "other", expires: time.Now().Add(time.Hour)}
	store.mu.Unlock()
	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("session kept after losing the lease")
	}
	time.Sleep(50 * time.Millisecond)
	if n := dials.Load(); n != 1 {
		t.Errorf("dialed %d times, want 1: no new session without the lease", n)
	}
	cancel()
	<-done
}

func TestBotReleasesLeaseOnExit(t *testing.T) {
	ctx := context.Background()
	store := &MemoryStore{}
	var dials atomic.Int32
	sessions := make(chan *Client, 1)
	b := testBot(store, "me", &dials, sessions)
	rctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { b.Run(rctx); close(done) }()
	c := <-sessions
	cancel()
	<-done
	if c.Err() == nil {
		t.Error("session alive after Run returned")
	}
	if ok, _ := store.LeaseBot(ctx, "bot", "next", time.Minute); !ok {
		t.Error("lease still held after Run returned")
	}
}

func TestBotBudgetOutlivesBot(t *testing.T) {
	ctx := context.Background()
	store := &MemoryStore{}
	log := slog.New(slog.DiscardHandler)
	acc := Account{Username: "bot", Token: "t"}

	// A rate limit recorded by one process pauses the bot for the next one.
	NewBot(log, acc, store, "a").outcome(ctx, saltsKind, &source.RateLimitError{What: "gc"})
	wait, ok, err := NewBot(log, acc, store, "b").reserve(ctx, saltsKind, true)
	if err != nil || ok || wait < 9*time.Minute {
		t.Errorf("after a stored rate limit: ok=%v wait=%v err=%v; want a ~10m pause", ok, wait, err)
	}
	if _, ok, _ := NewBot(log, acc, store, "b").reserve(ctx, historyKind, true); !ok {
		t.Error("history budget paused by a salts rate limit")
	}
}
