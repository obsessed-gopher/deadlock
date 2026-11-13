package db

import (
	"context"
	"time"

	v1 "github.com/obsessed-gopher/deadlock/gen/go/deadlock/v1"
)

// Game times (s) and soul totals the match facts are sampled at.
var (
	SoulMarksS  = []uint32{300, 600, 900, 1200}
	SoulTargets = []uint32{10000, 20000, 30000}
)

// soulPoint is one stat snapshot of a player: net worth at game time t.
type soulPoint struct{ t, nw uint32 }

// soulsAt interpolates net worth at time t between snapshots (the curve
// starts at 0 souls at 0 s). 0 when t is past the last snapshot.
func soulsAt(pts []soulPoint, t uint32) uint32 {
	prev := soulPoint{}
	for _, p := range pts {
		if p.t >= t {
			if p.t == prev.t {
				return p.nw
			}
			// Signed: net worth can drop between snapshots (souls lost on death).
			d := (int64(p.nw) - int64(prev.nw)) * int64(t-prev.t) / int64(p.t-prev.t)
			return uint32(int64(prev.nw) + d)
		}
		prev = p
	}
	return 0
}

// timeToSouls interpolates the game time net worth first reached target.
// 0 when it never did.
func timeToSouls(pts []soulPoint, target uint32) uint32 {
	prev := soulPoint{}
	for _, p := range pts {
		if p.nw >= target {
			if p.nw == prev.nw {
				return p.t
			}
			return prev.t + uint32(uint64(p.t-prev.t)*uint64(target-prev.nw)/uint64(p.nw-prev.nw))
		}
		prev = p
	}
	return 0
}

// Rank badges are packed as tier * 10 + subtier, subtier 1..6.
const subtiers = 6

// badgeStep is a badge as a step on the linear rank scale; -1 if invalid.
func badgeStep(b int16) int {
	tier, sub := int(b)/10, int(b)%10
	if tier < 1 || sub < 1 || sub > subtiers {
		return -1
	}
	return (tier-1)*subtiers + sub - 1
}

// lobbyBadge averages the reported team badges on the linear rank scale
// (averaging packed values would yield subtiers that do not exist), rounding
// half up. 0 when no team badge is valid.
func lobbyBadge(b0, b1 int16) uint32 {
	sum, n := 0, 0
	for _, b := range []int16{b0, b1} {
		if s := badgeStep(b); s >= 0 {
			sum += s
			n++
		}
	}
	if n == 0 {
		return 0
	}
	step := (2*sum + n) / (2 * n)
	return uint32((step/subtiers+1)*10 + step%subtiers + 1)
}

// ListPlayerMatchFacts returns compact facts of all matches of a player,
// oldest first. Matches without metadata have only the history fields. An
// unknown player has no rows.
func (d *DB) ListPlayerMatchFacts(ctx context.Context, accountID uint32) ([]*v1.MatchFacts, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT pm.match_id, m.start_time, m.duration_s, m.game_mode, m.match_mode, pm.hero_id, pm.won,
		       pm.kills, pm.deaths, pm.assists, pm.net_worth, pm.last_hits,
		       mp.slot IS NOT NULL, coalesce(mp.player_damage, 0), coalesce(mp.player_healing, 0),
		       m.badge_team0, m.badge_team1
		FROM player_matches pm
		JOIN matches m ON m.match_id = pm.match_id
		LEFT JOIN match_players mp ON mp.match_id = pm.match_id AND mp.account_id = pm.account_id
		WHERE pm.account_id = $1
		ORDER BY m.start_time, pm.match_id`, accountID)
	if err != nil {
		return nil, err
	}
	var (
		out  []*v1.MatchFacts
		byID = map[uint64]*v1.MatchFacts{}
	)
	for rows.Next() {
		var (
			f      v1.MatchFacts
			start  time.Time
			b0, b1 int16
		)
		if err := rows.Scan(&f.MatchId, &start, &f.DurationS, &f.GameMode, &f.MatchMode, &f.HeroId, &f.Won,
			&f.Kills, &f.Deaths, &f.Assists, &f.NetWorth, &f.LastHits,
			&f.HasMeta, &f.PlayerDamage, &f.PlayerHealing, &b0, &b1); err != nil {
			rows.Close()
			return nil, err
		}
		f.StartTime = start.Unix()
		if f.HasMeta {
			f.LobbyBadge = lobbyBadge(b0, b1)
		}
		out = append(out, &f)
		byID[f.MatchId] = &f
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// The player's own stat snapshots in every match with metadata.
	rows, err = d.Pool.Query(ctx, `
		SELECT ps.match_id, ps.t, ps.net_worth,
		       ps.gold_player, ps.gold_lane_creep, ps.gold_neutral_creep, ps.gold_boss, ps.gold_treasure
		FROM match_players mp
		JOIN player_stats ps ON ps.match_id = mp.match_id AND ps.slot = mp.slot
		WHERE mp.account_id = $1
		ORDER BY ps.match_id, ps.t`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var (
		cur  *v1.MatchFacts
		pts  []soulPoint
		last *v1.SoulSources // sources at the latest snapshot = whole match
	)
	flush := func() {
		if cur == nil {
			return
		}
		for _, t := range SoulMarksS {
			cur.SoulsAt = append(cur.SoulsAt, soulsAt(pts, t))
		}
		for _, s := range SoulTargets {
			cur.TimeToSoulsS = append(cur.TimeToSoulsS, timeToSouls(pts, s))
		}
		cur.SoulsFrom = last
	}
	for rows.Next() {
		var (
			id    uint64
			p     soulPoint
			src   = &v1.SoulSources{}
			match *v1.MatchFacts
		)
		if err := rows.Scan(&id, &p.t, &p.nw,
			&src.Heroes, &src.LaneCreeps, &src.Neutrals, &src.Bosses, &src.Treasure); err != nil {
			return nil, err
		}
		if match = byID[id]; match == nil {
			continue // a match of the player in metadata but not in their history
		}
		if match != cur {
			flush()
			cur, pts = match, pts[:0]
		}
		pts = append(pts, p)
		last = src
	}
	flush()
	return out, rows.Err()
}
