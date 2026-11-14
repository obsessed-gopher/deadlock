package steamgc

import (
	"context"
	"sync"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/source"
)

// Store keeps what must outlive a process and be shared between processes:
// the bots' request budgets and which process runs which bot (a Steam account
// holds one game session, so a bot logged on twice kicks itself off).
type Store interface {
	// UpdateBotBudget runs fn on the stored budget of one kind of one bot,
	// atomically, starting from init if there is none yet, and saves it.
	UpdateBotBudget(ctx context.Context, bot, kind string, init source.BotBudget, fn func(*source.BotBudget)) error
	// LeaseBot takes or renews the right to run the bot for ttl; false means
	// another owner holds an unexpired lease.
	LeaseBot(ctx context.Context, bot, owner string, ttl time.Duration) (bool, error)
	// ReleaseBot gives the lease up if owner holds it.
	ReleaseBot(ctx context.Context, bot, owner string) error
}

// MemoryStore is a Store of one process (tests, tools): nothing survives it.
type MemoryStore struct {
	mu      sync.Mutex
	budgets map[[2]string]source.BotBudget
	leases  map[string]memLease
}

type memLease struct {
	owner   string
	expires time.Time
}

// UpdateBotBudget implements Store.
func (m *MemoryStore) UpdateBotBudget(_ context.Context, bot, kind string, init source.BotBudget, fn func(*source.BotBudget)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.budgets == nil {
		m.budgets = map[[2]string]source.BotBudget{}
	}
	s, ok := m.budgets[[2]string{bot, kind}]
	if !ok {
		s = init
	}
	fn(&s)
	m.budgets[[2]string{bot, kind}] = s
	return nil
}

// LeaseBot implements Store.
func (m *MemoryStore) LeaseBot(_ context.Context, bot, owner string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.leases == nil {
		m.leases = map[string]memLease{}
	}
	now := time.Now()
	if l, ok := m.leases[bot]; ok && l.owner != owner && now.Before(l.expires) {
		return false, nil
	}
	m.leases[bot] = memLease{owner: owner, expires: now.Add(ttl)}
	return true, nil
}

// ReleaseBot implements Store.
func (m *MemoryStore) ReleaseBot(_ context.Context, bot, owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if l, ok := m.leases[bot]; ok && l.owner == owner {
		delete(m.leases, bot)
	}
	return nil
}
