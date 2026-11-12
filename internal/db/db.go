// Package db owns the Postgres connection pool, schema migrations and all
// SQL. Queries are hand-written; no ORM or query builder.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps a pgx pool.
type DB struct {
	Pool *pgxpool.Pool
}

// Open connects and verifies the connection.
func Open(ctx context.Context, url string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// Close releases all connections.
func (d *DB) Close() { d.Pool.Close() }

// Ping checks that the database is reachable.
func (d *DB) Ping(ctx context.Context) error { return d.Pool.Ping(ctx) }

// Migrate applies pending migrations from migrations/NNNN_name.sql in order,
// each in its own transaction. An advisory lock makes concurrent starts safe.
func (d *DB) Migrate(ctx context.Context) error {
	conn, err := d.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	const lockID = 0x0dead10c
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockID); err != nil {
		return fmt.Errorf("migrate: lock: %w", err)
	}
	// Best effort: the lock is also released when the connection closes.
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, lockID) }()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    integer PRIMARY KEY,
		name       text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	applied := map[int]bool{}
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	files, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		name := strings.TrimPrefix(f, "migrations/")
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("migrate: bad file name %s", name)
		}
		if applied[version] {
			continue
		}
		sql, err := migrationsFS.ReadFile(f)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, version, name)
			return err
		})
		if err != nil {
			return fmt.Errorf("migrate %s: %w", name, err)
		}
	}
	return nil
}
