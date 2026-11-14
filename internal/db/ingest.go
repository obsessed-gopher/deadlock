package db

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/obsessed-gopher/deadlock/internal/pb"
	"github.com/obsessed-gopher/deadlock/internal/source"

	"github.com/jackc/pgx/v5"
)

// Meta statuses stored in matches.meta_status.
const (
	MetaPending = "pending"
	MetaOK      = "ok"
	MetaExpired = "expired"
	MetaFailed  = "failed"
)

// EnsurePlayer creates the player row if missing.
func (d *DB) EnsurePlayer(ctx context.Context, accountID uint32) error {
	_, err := d.Pool.Exec(ctx, `INSERT INTO players (account_id) VALUES ($1) ON CONFLICT DO NOTHING`, accountID)
	return err
}

// SetSyncState updates a player's sync bookkeeping.
func (d *DB) SetSyncState(ctx context.Context, accountID uint32, state, errMsg string, synced bool) error {
	_, err := d.Pool.Exec(ctx, `
		UPDATE players SET sync_state = $2, sync_error = $3,
		       last_synced_at = CASE WHEN $4 THEN now() ELSE last_synced_at END
		WHERE account_id = $1`, accountID, state, errMsg, synced)
	return err
}

// UpsertHistory stores match history rows for a player. Returns how many
// matches were not known before (and are now queued for metadata).
func (d *DB) UpsertHistory(ctx context.Context, accountID uint32, ms []source.MatchSummary) (newMatches int, err error) {
	err = pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		for _, m := range ms {
			tag, err := tx.Exec(ctx, `
				INSERT INTO matches (match_id, start_time, duration_s, game_mode, match_mode, winning_team)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (match_id) DO NOTHING`,
				m.MatchID, time.Unix(m.StartTime, 0), m.DurationS, m.GameMode, m.MatchMode, m.Result)
			if err != nil {
				return err
			}
			newMatches += int(tag.RowsAffected())

			_, err = tx.Exec(ctx, `
				INSERT INTO player_matches (account_id, match_id, hero_id, team, won, kills, deaths, assists,
				                            net_worth, last_hits, denies, hero_level, abandoned_time_s)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
				ON CONFLICT (account_id, match_id) DO UPDATE SET
				    hero_id = EXCLUDED.hero_id, team = EXCLUDED.team, won = EXCLUDED.won,
				    kills = EXCLUDED.kills, deaths = EXCLUDED.deaths, assists = EXCLUDED.assists,
				    net_worth = EXCLUDED.net_worth, last_hits = EXCLUDED.last_hits, denies = EXCLUDED.denies,
				    hero_level = EXCLUDED.hero_level, abandoned_time_s = EXCLUDED.abandoned_time_s`,
				accountID, m.MatchID, m.HeroID, m.Team, m.Won(), m.Kills, m.Deaths, m.Assists,
				m.NetWorth, m.LastHits, m.Denies, m.HeroLevel, m.AbandonedTimeS)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return newMatches, err
}

// ClaimPendingMeta takes up to limit matches whose metadata is due for
// download, newest first (recent replays are the ones still hosted by Valve),
// and leases them: they are not due again for lease, so several workers (in
// one process or many) never take the same match. Storing, deferring or
// failing a match ends its lease; a worker that dies lets it expire.
func (d *DB) ClaimPendingMeta(ctx context.Context, limit int, lease time.Duration) ([]source.QueuedMatch, error) {
	rows, err := d.Pool.Query(ctx, `
		WITH due AS (
			SELECT match_id FROM matches
			WHERE meta_status = 'pending' AND meta_next_try <= now()
			ORDER BY match_id DESC LIMIT $1
			FOR UPDATE SKIP LOCKED)
		UPDATE matches m SET meta_next_try = now() + make_interval(secs => $2)
		FROM due WHERE m.match_id = due.match_id
		RETURNING m.match_id, m.start_time`, limit, lease.Seconds())
	if err != nil {
		return nil, err
	}
	ms, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (source.QueuedMatch, error) {
		var q source.QueuedMatch
		return q, r.Scan(&q.MatchID, &q.StartTime)
	})
	// UPDATE ... RETURNING has no order.
	slices.SortFunc(ms, func(a, b source.QueuedMatch) int { return cmp.Compare(b.MatchID, a.MatchID) })
	return ms, err
}

// CountPendingMeta returns how many of the player's matches are still waiting
// for metadata.
func (d *DB) CountPendingMeta(ctx context.Context, accountID uint32) (int, error) {
	var n int
	err := d.Pool.QueryRow(ctx, `
		SELECT count(*) FROM player_matches pm
		JOIN matches m ON m.match_id = pm.match_id
		WHERE pm.account_id = $1 AND m.meta_status = 'pending'`, accountID).Scan(&n)
	return n, err
}

// ResetInterruptedSyncs marks syncs left in the "running" state as failed and
// returns how many there were. A sync only stays "running" if the process
// died in the middle of it, so this is called once at startup, before any
// sync can begin.
func (d *DB) ResetInterruptedSyncs(ctx context.Context) (int, error) {
	tag, err := d.Pool.Exec(ctx, `
		UPDATE players SET sync_state = 'error', sync_error = 'interrupted by restart'
		WHERE sync_state = 'running'`)
	return int(tag.RowsAffected()), err
}

// MarkMetaFailure records a failed attempt. Permanent failures (expired
// replay) stop retries; transient ones back off and give up after maxAttempts.
func (d *DB) MarkMetaFailure(ctx context.Context, matchID uint64, expired bool, cause error, maxAttempts int) error {
	_, err := d.Pool.Exec(ctx, `
		UPDATE matches SET
		    meta_attempts = meta_attempts + 1,
		    meta_error = $2,
		    meta_status = CASE
		        WHEN $3 THEN 'expired'
		        WHEN meta_attempts + 1 >= $4 THEN 'failed'
		        ELSE 'pending' END,
		    meta_next_try = now() + make_interval(mins => 5 * power(2, meta_attempts)::int)
		WHERE match_id = $1`, matchID, cause.Error(), expired, maxAttempts)
	return err
}

// DeferMeta postpones a match's download until `until` without counting an
// attempt: the provider asked us to wait (rate limit), the match itself is
// fine.
func (d *DB) DeferMeta(ctx context.Context, matchID uint64, until time.Time, cause error) error {
	_, err := d.Pool.Exec(ctx, `
		UPDATE matches SET meta_error = $2, meta_next_try = $3
		WHERE match_id = $1 AND meta_status = 'pending'`, matchID, cause.Error(), until)
	return err
}

// DeriveVersion is the version of what StoreMeta derives from metadata. Bump
// it when the derivation changes (a new column, a fix): on start the syncer
// re-derives every stored match from meta_raw, without downloads.
const DeriveVersion = 1

// StoreMeta saves the raw metadata and (re)derives all per-player tables in
// one transaction. Safe to call again for the same match.
func (d *DB) StoreMeta(ctx context.Context, matchID uint64, raw []byte, version uint32, mi *pb.CMsgMatchMetaDataContents_MatchInfo) error {
	if mi.GetMatchId() != 0 && mi.GetMatchId() != matchID {
		return fmt.Errorf("store meta: file is for match %d, expected %d", mi.GetMatchId(), matchID)
	}
	return pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE matches SET
			    meta_status = 'ok', meta_error = '', meta_fetched_at = now(),
			    meta_raw = $2, meta_version = $3
			WHERE match_id = $1`, matchID, raw, version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("store meta: match row missing")
		}
		return derive(ctx, tx, matchID, mi)
	})
}

// NextStaleMeta returns up to limit ingested matches with id > after whose
// derived tables are older than DeriveVersion, by match id.
func (d *DB) NextStaleMeta(ctx context.Context, after uint64, limit int) ([]uint64, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT match_id FROM matches
		WHERE meta_status = 'ok' AND meta_raw IS NOT NULL AND derive_version < $1 AND match_id > $2
		ORDER BY match_id LIMIT $3`, DeriveVersion, after, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uint64])
}

// MetaRaw returns the stored .meta file of a match exactly as downloaded, or
// ErrNotFound.
func (d *DB) MetaRaw(ctx context.Context, matchID uint64) ([]byte, error) {
	var raw []byte
	err := d.Pool.QueryRow(ctx, `SELECT meta_raw FROM matches WHERE match_id = $1 AND meta_raw IS NOT NULL`, matchID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return raw, err
}

// RederiveMeta rebuilds the derived tables of an ingested match from its
// decoded meta_raw. meta_raw itself is never touched.
func (d *DB) RederiveMeta(ctx context.Context, matchID uint64, mi *pb.CMsgMatchMetaDataContents_MatchInfo) error {
	if mi.GetMatchId() != 0 && mi.GetMatchId() != matchID {
		return fmt.Errorf("rederive: meta is for match %d, expected %d", mi.GetMatchId(), matchID)
	}
	return pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		return derive(ctx, tx, matchID, mi)
	})
}

// derive writes everything computed from metadata: the match header fields
// it refines and all per-player tables.
func derive(ctx context.Context, tx pgx.Tx, matchID uint64, mi *pb.CMsgMatchMetaDataContents_MatchInfo) error {
	tag, err := tx.Exec(ctx, `
		UPDATE matches SET
		    duration_s = $2, winning_team = $3, game_mode = $4, match_mode = $5,
		    start_time = CASE WHEN $6::bigint > 0 THEN to_timestamp($6::bigint) ELSE start_time END,
		    badge_team0 = $7, badge_team1 = $8, derive_version = $9
		WHERE match_id = $1`,
		matchID, mi.GetDurationS(), int32(mi.GetWinningTeam()), int32(mi.GetGameMode()),
		int32(mi.GetMatchMode()), int64(mi.GetStartTime()),
		int16(mi.GetAverageBadgeTeam0()), int16(mi.GetAverageBadgeTeam1()), DeriveVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("derive: match row missing")
	}

	for _, t := range []string{"match_players", "player_stats", "item_events", "deaths"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+t+` WHERE match_id = $1`, matchID); err != nil {
			return err
		}
	}

	var players, stats, items, deaths [][]any
	for _, p := range mi.GetPlayers() {
		slot := int16(p.GetPlayerSlot())
		var dmg, heal uint32
		if ss := p.GetStats(); len(ss) > 0 {
			last := ss[len(ss)-1]
			dmg, heal = last.GetPlayerDamage(), last.GetPlayerHealing()
		}
		players = append(players, []any{
			matchID, slot, int64(p.GetAccountId()), int16(p.GetTeam()), int32(p.GetHeroId()),
			int16(p.GetAssignedLane()), int32(p.GetKills()), int32(p.GetDeaths()), int32(p.GetAssists()),
			int32(p.GetNetWorth()), int32(p.GetLastHits()), int32(p.GetDenies()), int32(p.GetLevel()),
			int32(dmg), int32(heal), int32(p.GetAbandonMatchTimeS()),
		})
		for _, s := range p.GetStats() {
			stats = append(stats, []any{
				matchID, slot, int32(s.GetTimeStampS()), int32(s.GetNetWorth()), int32(s.GetLevel()),
				int32(s.GetKills()), int32(s.GetDeaths()), int32(s.GetAssists()),
				int32(s.GetCreepKills()), int32(s.GetNeutralKills()), int32(s.GetDenies()),
				int32(s.GetPlayerDamage()), int32(s.GetPlayerDamageTaken()),
				int32(s.GetPlayerHealing()), int32(s.GetSelfHealing()),
				int32(s.GetGoldPlayer()), int32(s.GetGoldLaneCreep()), int32(s.GetGoldNeutralCreep()),
				int32(s.GetGoldBoss()), int32(s.GetGoldTreasure()), int32(s.GetGoldDeathLoss()),
				int32(s.GetShotsHit()), int32(s.GetShotsMissed()),
			})
		}
		for i, it := range p.GetItems() {
			items = append(items, []any{
				matchID, slot, int16(i), int32(it.GetGameTimeS()), int64(it.GetItemId()),
				int32(it.GetSoldTimeS()), int64(it.GetUpgradeId()), int32(it.GetFlags()),
			})
		}
		for i, dd := range p.GetDeathDetails() {
			pos, kp := dd.GetDeathPos(), dd.GetKillerPos()
			deaths = append(deaths, []any{
				matchID, slot, int16(i), int32(dd.GetGameTimeS()), int16(dd.GetKillerPlayerSlot()),
				pos.GetX(), pos.GetY(), pos.GetZ(), kp.GetX(), kp.GetY(), kp.GetZ(),
				int32(dd.GetDeathDurationS()), dd.GetTimeToKillS(),
			})
		}
	}

	copies := []struct {
		table string
		cols  []string
		rows  [][]any
	}{
		{"match_players", []string{
			"match_id", "slot", "account_id", "team", "hero_id", "lane", "kills", "deaths",
			"assists", "net_worth", "last_hits", "denies", "level", "player_damage", "player_healing", "abandon_time_s",
		}, players},
		{"player_stats", []string{
			"match_id", "slot", "t", "net_worth", "level", "kills", "deaths", "assists",
			"creep_kills", "neutral_kills", "denies", "player_damage", "player_damage_taken", "player_healing",
			"self_healing", "gold_player", "gold_lane_creep", "gold_neutral_creep", "gold_boss", "gold_treasure",
			"gold_death_loss", "shots_hit", "shots_missed",
		}, stats},
		{"item_events", []string{"match_id", "slot", "seq", "t", "item_id", "sold_t", "upgrade_id", "flags"}, items},
		{"deaths", []string{
			"match_id", "slot", "seq", "t", "killer_slot", "x", "y", "z",
			"killer_x", "killer_y", "killer_z", "respawn_s", "ttk_s",
		}, deaths},
	}
	for _, c := range copies {
		if len(c.rows) == 0 {
			continue
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{c.table}, c.cols, pgx.CopyFromRows(c.rows)); err != nil {
			return fmt.Errorf("copy %s: %w", c.table, err)
		}
	}
	return nil
}
