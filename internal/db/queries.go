package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	v1 "github.com/obsessed-gopher/deadlock/gen/go/deadlock/v1"
)

// ErrNotFound is returned when the requested row doesn't exist.
var ErrNotFound = errors.New("not found")

var metaStatusToAPI = map[string]v1.MetaStatus{
	MetaPending: v1.MetaStatus_META_STATUS_PENDING,
	MetaOK:      v1.MetaStatus_META_STATUS_OK,
	MetaExpired: v1.MetaStatus_META_STATUS_EXPIRED,
	MetaFailed:  v1.MetaStatus_META_STATUS_FAILED,
}

var metaStatusFromAPI = map[v1.MetaStatus]string{
	v1.MetaStatus_META_STATUS_PENDING: MetaPending,
	v1.MetaStatus_META_STATUS_OK:      MetaOK,
	v1.MetaStatus_META_STATUS_EXPIRED: MetaExpired,
	v1.MetaStatus_META_STATUS_FAILED:  MetaFailed,
}

var syncStateToAPI = map[string]v1.SyncState{
	"never":   v1.SyncState_SYNC_STATE_NEVER,
	"running": v1.SyncState_SYNC_STATE_RUNNING,
	"idle":    v1.SyncState_SYNC_STATE_IDLE,
	"error":   v1.SyncState_SYNC_STATE_ERROR,
}

// GetPlayer returns the profile and match counters of a tracked player, or
// ErrNotFound.
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

// MatchQuery selects a page of a player's match history.
type MatchQuery struct {
	AccountID uint32

	// Filters; zero values mean "any".
	HeroID     uint32
	Result     v1.MatchResult
	GameMode   int32
	MetaStatus v1.MetaStatus
	Queue      v1.MatchQueue

	Sort      v1.MatchSort // UNSPECIFIED = START_TIME
	Ascending bool
	// After continues a previous page: rows strictly after this
	// (sort value, match id) in the requested order. Nil = first page.
	After *MatchCursor
	Limit int
}

// MatchCursor is the position of the last row of a page.
type MatchCursor struct {
	Value   int64
	MatchID uint64
}

// matchSortExprs maps a sort key to its SQL expression. Only these constants
// are ever concatenated into the query; values always go as parameters.
var matchSortExprs = map[v1.MatchSort]string{
	v1.MatchSort_MATCH_SORT_UNSPECIFIED: "extract(epoch FROM m.start_time)::bigint",
	v1.MatchSort_MATCH_SORT_START_TIME:  "extract(epoch FROM m.start_time)::bigint",
	v1.MatchSort_MATCH_SORT_DURATION:    "m.duration_s",
	v1.MatchSort_MATCH_SORT_KILLS:       "pm.kills",
	v1.MatchSort_MATCH_SORT_DEATHS:      "pm.deaths",
	v1.MatchSort_MATCH_SORT_ASSISTS:     "pm.assists",
	v1.MatchSort_MATCH_SORT_NET_WORTH:   "pm.net_worth",
	v1.MatchSort_MATCH_SORT_LAST_HITS:   "pm.last_hits",
	v1.MatchSort_MATCH_SORT_HERO_LEVEL:  "pm.hero_level",
}

// MatchSortValue is the value of a row that a cursor stores for this sort.
func MatchSortValue(r *v1.PlayerMatchRow, s v1.MatchSort) int64 {
	switch s {
	case v1.MatchSort_MATCH_SORT_DURATION:
		return int64(r.GetDurationS())
	case v1.MatchSort_MATCH_SORT_KILLS:
		return int64(r.GetKills())
	case v1.MatchSort_MATCH_SORT_DEATHS:
		return int64(r.GetDeaths())
	case v1.MatchSort_MATCH_SORT_ASSISTS:
		return int64(r.GetAssists())
	case v1.MatchSort_MATCH_SORT_NET_WORTH:
		return int64(r.GetNetWorth())
	case v1.MatchSort_MATCH_SORT_LAST_HITS:
		return int64(r.GetLastHits())
	case v1.MatchSort_MATCH_SORT_HERO_LEVEL:
		return int64(r.GetHeroLevel())
	default:
		return r.GetStartTime()
	}
}

// ErrBadQuery is returned for a sort or filter value the query does not know.
var ErrBadQuery = errors.New("bad query")

// matchFilter is the shared WHERE clause of ListPlayerMatches and its count.
const matchFilter = `
		WHERE pm.account_id = $1
		  AND ($2 = 0 OR pm.hero_id = $2)
		  AND ($3 = 0 OR pm.won = ($3 = 1))
		  AND ($4 = 0 OR m.game_mode = $4)
		  AND ($5 = '' OR m.meta_status = $5)
		  AND ($6 = 0 OR ` + queueOf + ` = $6)`

// queueOf is the SQL of v1.MatchQueue for a match row m: ranked is a match
// mode (4) of the normal game mode (1); placement (8) counts as normal;
// game mode 4 is Street Brawl. Keep in sync with queueOf in web/src/lib.
const queueOf = `CASE
		      WHEN m.game_mode = 4 THEN 3
		      WHEN m.game_mode = 1 AND m.match_mode = 4 THEN 2
		      WHEN m.game_mode = 1 AND m.match_mode IN (1, 8) THEN 1
		      ELSE 4 END`

// ListPlayerMatches returns one page of a player's matches and the number of
// rows matching the filters across all pages. Pages are keyset by
// (sort value, match_id), so they stay stable while new matches arrive.
func (d *DB) ListPlayerMatches(ctx context.Context, q MatchQuery) ([]*v1.PlayerMatchRow, int, error) {
	expr, ok := matchSortExprs[q.Sort]
	if !ok {
		return nil, 0, fmt.Errorf("%w: sort %v", ErrBadQuery, q.Sort)
	}
	meta := ""
	if q.MetaStatus != v1.MetaStatus_META_STATUS_UNSPECIFIED {
		if meta, ok = metaStatusFromAPI[q.MetaStatus]; !ok {
			return nil, 0, fmt.Errorf("%w: meta status %v", ErrBadQuery, q.MetaStatus)
		}
	}
	if q.Result < v1.MatchResult_MATCH_RESULT_UNSPECIFIED || q.Result > v1.MatchResult_MATCH_RESULT_LOSS {
		return nil, 0, fmt.Errorf("%w: result %v", ErrBadQuery, q.Result)
	}
	if _, ok := v1.MatchQueue_name[int32(q.Queue)]; !ok {
		return nil, 0, fmt.Errorf("%w: queue %v", ErrBadQuery, q.Queue)
	}
	args := []any{q.AccountID, q.HeroID, int32(q.Result), q.GameMode, meta, int32(q.Queue)}

	var total int
	if err := d.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM player_matches pm
		JOIN matches m ON m.match_id = pm.match_id`+matchFilter, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	dir, cmp := "DESC", "<"
	if q.Ascending {
		dir, cmp = "ASC", ">"
	}
	cursor := ""
	if q.After != nil {
		cursor = " AND (" + expr + ", m.match_id) " + cmp + " ($7, $8)"
		args = append(args, q.After.Value, q.After.MatchID)
	}
	args = append(args, q.Limit)
	sql := `
		SELECT m.match_id, m.start_time, m.duration_s, m.game_mode, m.match_mode,
		       pm.hero_id, pm.team, pm.won, pm.kills, pm.deaths, pm.assists,
		       pm.net_worth, pm.last_hits, pm.denies, pm.hero_level, m.meta_status
		FROM player_matches pm
		JOIN matches m ON m.match_id = pm.match_id` + matchFilter + cursor + `
		ORDER BY ` + expr + ` ` + dir + `, m.match_id ` + dir + `
		LIMIT $` + strconv.Itoa(len(args))
	rows, err := d.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
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
			return nil, 0, err
		}
		r.StartTime = start.Unix()
		r.MetaStatus = metaStatusToAPI[status]
		out = append(out, &r)
	}
	return out, total, rows.Err()
}

// ListPlayerHeroStats sums a player's match history per hero, most played
// first. It needs only match history, not metadata. An unknown player has no
// rows.
func (d *DB) ListPlayerHeroStats(ctx context.Context, accountID uint32) ([]*v1.HeroStats, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT pm.hero_id, count(*), count(*) FILTER (WHERE pm.won),
		       sum(pm.kills), sum(pm.deaths), sum(pm.assists),
		       sum(pm.net_worth), sum(pm.last_hits), sum(m.duration_s), max(m.start_time)
		FROM player_matches pm
		JOIN matches m ON m.match_id = pm.match_id
		WHERE pm.account_id = $1
		GROUP BY pm.hero_id
		ORDER BY count(*) DESC, pm.hero_id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*v1.HeroStats
	for rows.Next() {
		var (
			h    v1.HeroStats
			last time.Time
		)
		if err := rows.Scan(&h.HeroId, &h.Matches, &h.Wins, &h.Kills, &h.Deaths, &h.Assists,
			&h.NetWorth, &h.LastHits, &h.DurationS, &last); err != nil {
			return nil, err
		}
		h.LastPlayed = last.Unix()
		out = append(out, &h)
	}
	return out, rows.Err()
}

// GetMatch returns the match header and, once metadata is ingested, the
// scoreboard of all players. Unknown matches yield ErrNotFound.
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

// GetPlayerMatch returns one player's detailed breakdown of a match. It yields
// ErrNotFound when the match is unknown or the player did not take part.
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
