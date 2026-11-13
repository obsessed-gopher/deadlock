package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/obsessed-gopher/deadlock/internal/source"

	v1 "github.com/obsessed-gopher/deadlock/gen/go/deadlock/v1"
)

// StaleProfiles returns accounts whose Steam profile was never fetched or is
// older than maxAge: tracked players first, then everyone from stored
// matches, never-fetched before outdated.
func (d *DB) StaleProfiles(ctx context.Context, maxAge time.Duration, limit int) ([]uint32, error) {
	rows, err := d.Pool.Query(ctx, `
		WITH seen AS (
			SELECT account_id, true AS tracked FROM players
			UNION ALL
			SELECT DISTINCT account_id, false FROM match_players WHERE account_id <> 0
		), accounts AS (
			SELECT account_id, bool_or(tracked) AS tracked FROM seen GROUP BY account_id
		)
		SELECT a.account_id
		FROM accounts a
		LEFT JOIN steam_profiles s ON s.account_id = a.account_id
		WHERE s.fetched_at IS NULL OR s.fetched_at < now() - $1::interval
		ORDER BY a.tracked DESC, s.fetched_at NULLS FIRST, a.account_id
		LIMIT $2`, maxAge, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uint32])
}

// StoreProfiles saves the answer for the requested accounts: found profiles
// are upserted, the rest are remembered as not found. Friend lists are kept
// only for tracked players.
func (d *DB) StoreProfiles(ctx context.Context, requested []uint32, got []source.SteamProfile, at time.Time) error {
	return pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		found := make(map[uint32]bool, len(got))
		for _, p := range got {
			found[p.AccountID] = true
			country := ""
			if p.CountryCode != nil {
				country = *p.CountryCode
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO steam_profiles (account_id, found, persona_name, profile_url, avatar, avatar_medium,
				                            avatar_full, country, fetched_at)
				VALUES ($1, true, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (account_id) DO UPDATE SET
				    found = true, persona_name = EXCLUDED.persona_name, profile_url = EXCLUDED.profile_url,
				    avatar = EXCLUDED.avatar, avatar_medium = EXCLUDED.avatar_medium,
				    avatar_full = EXCLUDED.avatar_full, country = EXCLUDED.country, fetched_at = EXCLUDED.fetched_at`,
				p.AccountID, p.PersonaName, p.ProfileURL, p.Avatar, p.AvatarMedium, p.AvatarFull, country, at); err != nil {
				return err
			}
			var tracked bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM players WHERE account_id = $1)`, p.AccountID).Scan(&tracked); err != nil {
				return err
			}
			if !tracked {
				continue
			}
			if _, err := tx.Exec(ctx, `DELETE FROM steam_friends WHERE account_id = $1`, p.AccountID); err != nil {
				return err
			}
			for _, f := range p.Friends {
				if _, err := tx.Exec(ctx, `
					INSERT INTO steam_friends (account_id, friend_id, friend_since) VALUES ($1, $2, $3)
					ON CONFLICT DO NOTHING`, p.AccountID, f.AccountID, f.FriendSince); err != nil {
					return err
				}
			}
		}
		for _, id := range requested {
			if found[id] {
				continue
			}
			// Keep the last known name: a profile that vanished from the API
			// is more likely a hiccup than a deleted account.
			if _, err := tx.Exec(ctx, `
				INSERT INTO steam_profiles (account_id, found, fetched_at) VALUES ($1, false, $2)
				ON CONFLICT (account_id) DO UPDATE SET fetched_at = EXCLUDED.fetched_at`, id, at); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListProfiles returns the known profiles of the given accounts, in no
// particular order; unknown accounts are skipped.
func (d *DB) ListProfiles(ctx context.Context, ids []uint32) ([]*v1.Profile, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ids64 := make([]int64, len(ids))
	for i, id := range ids {
		ids64[i] = int64(id)
	}
	rows, err := d.Pool.Query(ctx, `
		SELECT account_id, persona_name, profile_url, avatar, avatar_medium, avatar_full, country
		FROM steam_profiles
		WHERE account_id = ANY($1) AND persona_name <> ''
		ORDER BY account_id`, ids64)
	if err != nil {
		return nil, err
	}
	var out []*v1.Profile
	for rows.Next() {
		var p v1.Profile
		if err := rows.Scan(&p.AccountId, &p.PersonaName, &p.ProfileUrl, &p.Avatar, &p.AvatarMedium, &p.AvatarFull, &p.Country); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}
