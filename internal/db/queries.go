package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	v1 "deadlock_ideas/gen/go/deadlock/v1"
)

// ErrNotFound is returned when the requested row doesn't exist.
var ErrNotFound = errors.New("not found")

var metaStatusToAPI = map[string]v1.MetaStatus{
	MetaPending: v1.MetaStatus_META_STATUS_PENDING,
	MetaOK:      v1.MetaStatus_META_STATUS_OK,
	MetaExpired: v1.MetaStatus_META_STATUS_EXPIRED,
	MetaFailed:  v1.MetaStatus_META_STATUS_FAILED,
}

var syncStateToAPI = map[string]v1.SyncState{
	"never":   v1.SyncState_SYNC_STATE_NEVER,
	"running": v1.SyncState_SYNC_STATE_RUNNING,
	"idle":    v1.SyncState_SYNC_STATE_IDLE,
	"error":   v1.SyncState_SYNC_STATE_ERROR,
}

func (d *DB) GetPlayer(ctx context.Context, accountID uint32) (*v1.Player, error) {
	var (
		p      v1.Player
		state  string
		synced *time.Time
	)
	err := d.Pool.QueryRow(ctx, `
		SELECT p.account_id, p.sync_state, p.sync_error, p.last_synced_at,
		       count(pm.match_id),
		       count(pm.match_id) FILTER (WHERE m.meta_status = 'ok'),
		       count(pm.match_id) FILTER (WHERE m.meta_status = 'pending'),
		       count(pm.match_id) FILTER (WHERE pm.won)
		FROM players p
		LEFT JOIN player_matches pm ON pm.account_id = p.account_id
		LEFT JOIN matches m ON m.match_id = pm.match_id
		WHERE p.account_id = $1
		GROUP BY p.account_id`, accountID).
		Scan(&p.AccountId, &state, &p.SyncError, &synced,
			&p.MatchesTotal, &p.MatchesWithMeta, &p.MatchesPending, &p.Wins)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.SyncState = syncStateToAPI[state]
	if synced != nil {
		p.LastSyncedAt = synced.Unix()
	}
	return &p, nil
}

// ListPlayerMatches pages by match_id descending; beforeMatchID = 0 means
// start from the newest.
func (d *DB) ListPlayerMatches(ctx context.Context, accountID uint32, heroID uint32, beforeMatchID uint64, limit int) ([]*v1.PlayerMatchRow, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT m.match_id, m.start_time, m.duration_s, m.game_mode, m.match_mode,
		       pm.hero_id, pm.team, pm.won, pm.kills, pm.deaths, pm.assists,
		       pm.net_worth, pm.last_hits, pm.denies, pm.hero_level, m.meta_status
		FROM player_matches pm
		JOIN matches m ON m.match_id = pm.match_id
		WHERE pm.account_id = $1
		  AND ($2 = 0 OR pm.hero_id = $2)
		  AND ($3 = 0 OR m.match_id < $3)
		ORDER BY m.match_id DESC
		LIMIT $4`, accountID, heroID, beforeMatchID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*v1.PlayerMatchRow
	for rows.Next() {
		var (
			r      v1.PlayerMatchRow
			start  time.Time
			status string
		)
		if err := rows.Scan(&r.MatchId, &start, &r.DurationS, &r.GameMode, &r.MatchMode,
			&r.HeroId, &r.Team, &r.Won, &r.Kills, &r.Deaths, &r.Assists,
			&r.NetWorth, &r.LastHits, &r.Denies, &r.HeroLevel, &status); err != nil {
			return nil, err
		}
		r.StartTime = start.Unix()
		r.MetaStatus = metaStatusToAPI[status]
		out = append(out, &r)
	}
	return out, rows.Err()
}

func (d *DB) GetMatch(ctx context.Context, matchID uint64) (*v1.Match, error) {
	var (
		m      v1.Match
		start  time.Time
		status string
	)
	err := d.Pool.QueryRow(ctx, `
		SELECT match_id, start_time, duration_s, game_mode, match_mode, winning_team, meta_status
		FROM matches WHERE match_id = $1`, matchID).
		Scan(&m.MatchId, &start, &m.DurationS, &m.GameMode, &m.MatchMode, &m.WinningTeam, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.StartTime = start.Unix()
	m.MetaStatus = metaStatusToAPI[status]

	rows, err := d.Pool.Query(ctx, `
		SELECT slot, account_id, team, hero_id, lane, kills, deaths, assists, net_worth,
		       last_hits, denies, level, player_damage, player_healing, abandon_time_s
		FROM match_players WHERE match_id = $1 ORDER BY team, net_worth DESC`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p v1.MatchPlayer
		if err := rows.Scan(&p.Slot, &p.AccountId, &p.Team, &p.HeroId, &p.Lane, &p.Kills, &p.Deaths,
			&p.Assists, &p.NetWorth, &p.LastHits, &p.Denies, &p.Level, &p.PlayerDamage,
			&p.PlayerHealing, &p.AbandonTimeS); err != nil {
			return nil, err
		}
		m.Players = append(m.Players, &p)
	}
	return &m, rows.Err()
}

func (d *DB) GetPlayerMatch(ctx context.Context, matchID uint64, accountID uint32) (*v1.PlayerMatch, error) {
	m, err := d.GetMatch(ctx, matchID)
	if err != nil {
		return nil, err
	}
	out := &v1.PlayerMatch{Match: m}

	found := false
	for _, p := range m.Players {
		if p.AccountId == accountID {
			out.Slot, found = p.Slot, true
			break
		}
	}
	if !found {
		return nil, ErrNotFound
	}

	// All players' stat snapshots: full detail for the requested slot,
	// net worth only for the others.
	rows, err := d.Pool.Query(ctx, `
		SELECT slot, t, net_worth, level, kills, deaths, assists, creep_kills, neutral_kills, denies,
		       player_damage, player_damage_taken, player_healing, self_healing,
		       gold_player, gold_lane_creep, gold_neutral_creep, gold_boss, gold_treasure,
		       gold_death_loss, shots_hit, shots_missed
		FROM player_stats WHERE match_id = $1 ORDER BY slot, t`, matchID)
	if err != nil {
		return nil, err
	}
	bySlot := map[uint32]*v1.SlotTimeline{}
	for rows.Next() {
		var slot uint32
		s := &v1.StatSnapshot{}
		if err := rows.Scan(&slot, &s.T, &s.NetWorth, &s.Level, &s.Kills, &s.Deaths, &s.Assists,
			&s.CreepKills, &s.NeutralKills, &s.Denies, &s.PlayerDamage, &s.PlayerDamageTaken,
			&s.PlayerHealing, &s.SelfHealing, &s.GoldPlayer, &s.GoldLaneCreep, &s.GoldNeutralCreep,
			&s.GoldBoss, &s.GoldTreasure, &s.GoldDeathLoss, &s.ShotsHit, &s.ShotsMissed); err != nil {
			rows.Close()
			return nil, err
		}
		if slot == out.Slot {
			out.Timeline = append(out.Timeline, s)
		}
		tl := bySlot[slot]
		if tl == nil {
			tl = &v1.SlotTimeline{Slot: slot}
			bySlot[slot] = tl
			out.NetWorthBySlot = append(out.NetWorthBySlot, tl)
		}
		tl.Points = append(tl.Points, &v1.TimePoint{T: s.T, V: s.NetWorth})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = d.Pool.Query(ctx, `
		SELECT t, item_id, sold_t, upgrade_id, flags
		FROM item_events WHERE match_id = $1 AND slot = $2 ORDER BY seq`, matchID, out.Slot)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var it v1.ItemEvent
		var itemID, upgradeID int64
		if err := rows.Scan(&it.T, &itemID, &it.SoldT, &upgradeID, &it.Flags); err != nil {
			rows.Close()
			return nil, err
		}
		it.ItemId, it.UpgradeId = uint32(itemID), uint32(upgradeID)
		out.Items = append(out.Items, &it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = d.Pool.Query(ctx, `
		SELECT t, killer_slot, x, y, z, killer_x, killer_y, killer_z, respawn_s, ttk_s
		FROM deaths WHERE match_id = $1 AND slot = $2 ORDER BY seq`, matchID, out.Slot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		dd := v1.Death{Pos: &v1.Vec3{}, KillerPos: &v1.Vec3{}}
		if err := rows.Scan(&dd.T, &dd.KillerSlot, &dd.Pos.X, &dd.Pos.Y, &dd.Pos.Z,
			&dd.KillerPos.X, &dd.KillerPos.Y, &dd.KillerPos.Z, &dd.RespawnS, &dd.TimeToKillS); err != nil {
			return nil, err
		}
		out.Deaths = append(out.Deaths, &dd)
	}
	return out, rows.Err()
}
