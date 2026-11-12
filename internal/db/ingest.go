package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/obsessed-gopher/deadlock/internal/pb"
	"github.com/obsessed-gopher/deadlock/internal/source"
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

// NextPendingMeta returns matches whose metadata is due for download,
// newest first (recent replays are the ones still hosted by Valve).
func (d *DB) NextPendingMeta(ctx context.Context, limit int) ([]uint64, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT match_id FROM matches
		WHERE meta_status = 'pending' AND meta_next_try <= now()
		ORDER BY match_id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uint64])
}

// CountPendingMeta returns the number of matches still waiting for metadata.
func (d *DB) CountPendingMeta(ctx context.Context) (int, error) {
	var n int
	err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM matches WHERE meta_status = 'pending'`).Scan(&n)
	return n, err
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

// StoreMeta saves the raw metadata and (re)derives all per-player tables in
// one transaction. Safe to call again for the same match.
func (d *DB) StoreMeta(ctx context.Context, matchID uint64, raw []byte, version uint32, mi *pb.CMsgMatchMetaDataContents_MatchInfo) error {
	if mi.GetMatchId() != 0 && mi.GetMatchId() != matchID {
		return fmt.Errorf("store meta: file is for match %d, expected %d", mi.GetMatchId(), matchID)
	}
	return pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE matches SET
			    duration_s = $2, winning_team = $3, game_mode = $4, match_mode = $5,
			    start_time = CASE WHEN $6::bigint > 0 THEN to_timestamp($6::bigint) ELSE start_time END,
			    meta_status = 'ok', meta_error = '', meta_fetched_at = now(),
			    meta_raw = $7, meta_version = $8
			WHERE match_id = $1`,
			matchID, mi.GetDurationS(), int32(mi.GetWinningTeam()), int32(mi.GetGameMode()),
			int32(mi.GetMatchMode()), int64(mi.GetStartTime()), raw, version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("store meta: match row missing")
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
			{"match_players", []string{"match_id", "slot", "account_id", "team", "hero_id", "lane", "kills", "deaths",
				"assists", "net_worth", "last_hits", "denies", "level", "player_damage", "player_healing", "abandon_time_s"}, players},
			{"player_stats", []string{"match_id", "slot", "t", "net_worth", "level", "kills", "deaths", "assists",
				"creep_kills", "neutral_kills", "denies", "player_damage", "player_damage_taken", "player_healing",
				"self_healing", "gold_player", "gold_lane_creep", "gold_neutral_creep", "gold_boss", "gold_treasure",
				"gold_death_loss", "shots_hit", "shots_missed"}, stats},
			{"item_events", []string{"match_id", "slot", "seq", "t", "item_id", "sold_t", "upgrade_id", "flags"}, items},
			{"deaths", []string{"match_id", "slot", "seq", "t", "killer_slot", "x", "y", "z",
				"killer_x", "killer_y", "killer_z", "respawn_s", "ttk_s"}, deaths},
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
	})
}
