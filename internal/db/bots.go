package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/obsessed-gopher/deadlock/internal/source"
)

// UpdateBotBudget runs fn on the stored budget of one kind of one Steam bot
// under a row lock, starting from init if there is none, and saves the
// result: concurrent requests of any process see each other's spending.
func (d *DB) UpdateBotBudget(ctx context.Context, bot, kind string, init source.BotBudget, fn func(*source.BotBudget)) error {
	return pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO steam_bot_budgets (bot, kind, tokens, refilled_at, paused_until, strikes)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (bot, kind) DO NOTHING`,
			bot, kind, init.Tokens, init.RefilledAt, init.PausedUntil, init.Strikes); err != nil {
			return fmt.Errorf("bot budget: %w", err)
		}
		var s source.BotBudget
		if err := tx.QueryRow(ctx, `
			SELECT tokens, refilled_at, paused_until, strikes FROM steam_bot_budgets
			WHERE bot = $1 AND kind = $2 FOR UPDATE`, bot, kind).
			Scan(&s.Tokens, &s.RefilledAt, &s.PausedUntil, &s.Strikes); err != nil {
			return fmt.Errorf("bot budget: %w", err)
		}
		fn(&s)
		if _, err := tx.Exec(ctx, `
			UPDATE steam_bot_budgets
			SET tokens = $3, refilled_at = $4, paused_until = $5, strikes = $6, updated_at = now()
			WHERE bot = $1 AND kind = $2`,
			bot, kind, s.Tokens, s.RefilledAt, s.PausedUntil, s.Strikes); err != nil {
			return fmt.Errorf("bot budget: %w", err)
		}
		return nil
	})
}

// LeaseBot takes the right to run a Steam bot for ttl, or renews it: it
// succeeds when nobody holds the bot, owner already holds it, or the holder's
// lease expired. Expiry is judged by the database clock, the same for every
// process.
func (d *DB) LeaseBot(ctx context.Context, bot, owner string, ttl time.Duration) (bool, error) {
	var got string
	err := d.Pool.QueryRow(ctx, `
		INSERT INTO steam_bot_leases (bot, owner, expires_at)
		VALUES ($1, $2, now() + make_interval(secs => $3))
		ON CONFLICT (bot) DO UPDATE SET owner = excluded.owner, expires_at = excluded.expires_at
		WHERE steam_bot_leases.owner = excluded.owner OR steam_bot_leases.expires_at < now()
		RETURNING owner`, bot, owner, ttl.Seconds()).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("bot lease: %w", err)
	}
	return true, nil
}

// ReleaseBot ends owner's lease of the bot, if it holds one.
func (d *DB) ReleaseBot(ctx context.Context, bot, owner string) error {
	if _, err := d.Pool.Exec(ctx, `DELETE FROM steam_bot_leases WHERE bot = $1 AND owner = $2`, bot, owner); err != nil {
		return fmt.Errorf("bot lease: %w", err)
	}
	return nil
}
