package db

import (
	"context"
	"time"

	v1 "github.com/obsessed-gopher/deadlock/gen/go/deadlock/v1"
)

const (
	// PeerMinMatches is how often someone must be met to be listed: one
	// meeting is chance and would flood the lists with thousands of names.
	PeerMinMatches = 2
	// FriendMinMatches is how many shared matches on one team make a friend.
	// Opponents repeat at most a few times by chance (4 in 680 matches of
	// real data), so 5 separates queue mates from random people.
	FriendMinMatches = 5
)

// Peers is what ListPlayerPeers returns.
type Peers struct {
	Matches, Wins uint32
	Allies        []*v1.Peer
	Enemies       []*v1.Peer
	Heroes        []*v1.HeroMatchup
	PartySizes    []*v1.PartySize
}

// mine is the requested player's matches with metadata; $1 is the account.
const mine = `
	WITH me AS (
		SELECT mp.match_id, mp.team, m.winning_team = mp.team AS won, m.start_time
		FROM match_players mp
		JOIN matches m ON m.match_id = mp.match_id
		WHERE mp.account_id = $1
	)`

// ListPlayerPeers returns the people and heroes met in a player's matches
// with metadata. Rosters come from metadata, so this works for any player
// seen in a stored match, tracked or not; an unknown player gets zeros.
func (d *DB) ListPlayerPeers(ctx context.Context, accountID uint32) (*Peers, error) {
	var p Peers
	if err := d.Pool.QueryRow(ctx, mine+`
		SELECT count(*), count(*) FILTER (WHERE won) FROM me`, accountID).Scan(&p.Matches, &p.Wins); err != nil {
		return nil, err
	}
	if p.Matches == 0 {
		return &p, nil
	}

	rows, err := d.Pool.Query(ctx, mine+`
		SELECT o.account_id, o.team = me.team, count(*), count(*) FILTER (WHERE me.won),
		       min(me.start_time), max(me.start_time),
		       (array_agg(me.match_id ORDER BY me.start_time DESC, me.match_id DESC))[1],
		       mode() WITHIN GROUP (ORDER BY o.hero_id),
		       EXISTS (SELECT 1 FROM steam_friends sf WHERE sf.account_id = $1 AND sf.friend_id = o.account_id)
		FROM me
		JOIN match_players o ON o.match_id = me.match_id AND o.account_id <> $1 AND o.account_id <> 0
		GROUP BY o.account_id, o.team = me.team
		HAVING count(*) >= $2
		ORDER BY count(*) DESC, max(me.start_time) DESC, o.account_id`, accountID, PeerMinMatches)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var (
			r           v1.Peer
			ally        bool
			first, last time.Time
		)
		if err := rows.Scan(&r.AccountId, &ally, &r.Matches, &r.Wins, &first, &last, &r.LastMatchId, &r.TopHeroId, &r.SteamFriend); err != nil {
			rows.Close()
			return nil, err
		}
		r.FirstPlayed, r.LastPlayed = first.Unix(), last.Unix()
		if ally {
			p.Allies = append(p.Allies, &r)
		} else {
			p.Enemies = append(p.Enemies, &r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = d.Pool.Query(ctx, mine+`
		SELECT o.hero_id,
		       count(*) FILTER (WHERE o.team = me.team),
		       count(*) FILTER (WHERE o.team = me.team AND me.won),
		       count(*) FILTER (WHERE o.team <> me.team),
		       count(*) FILTER (WHERE o.team <> me.team AND me.won)
		FROM me
		JOIN match_players o ON o.match_id = me.match_id AND o.account_id <> $1
		GROUP BY o.hero_id
		ORDER BY count(*) DESC, o.hero_id`, accountID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var h v1.HeroMatchup
		if err := rows.Scan(&h.HeroId, &h.WithMatches, &h.WithWins, &h.AgainstMatches, &h.AgainstWins); err != nil {
			rows.Close()
			return nil, err
		}
		p.Heroes = append(p.Heroes, &h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = d.Pool.Query(ctx, mine+`,
	mates AS (
		SELECT me.match_id, o.account_id
		FROM me
		JOIN match_players o ON o.match_id = me.match_id AND o.team = me.team
		                    AND o.account_id <> $1 AND o.account_id <> 0
	),
	friends AS (
		SELECT account_id FROM mates GROUP BY account_id HAVING count(*) >= $2
	),
	party AS (
		SELECT me.match_id, me.won, count(f.account_id) AS friends
		FROM me
		LEFT JOIN mates ON mates.match_id = me.match_id
		LEFT JOIN friends f ON f.account_id = mates.account_id
		GROUP BY me.match_id, me.won
	)
	SELECT friends, count(*), count(*) FILTER (WHERE won)
	FROM party
	GROUP BY friends
	ORDER BY friends`, accountID, FriendMinMatches)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s v1.PartySize
		if err := rows.Scan(&s.Friends, &s.Matches, &s.Wins); err != nil {
			rows.Close()
			return nil, err
		}
		p.PartySizes = append(p.PartySizes, &s)
	}
	return &p, rows.Err()
}

// ListSeenMatches returns the player's line in every stored match with
// metadata, oldest first. Rosters come from metadata, so this works for any
// player seen in a match; an unknown one has no rows.
func (d *DB) ListSeenMatches(ctx context.Context, accountID uint32) ([]*v1.SeenMatch, error) {
	rows, err := d.Pool.Query(ctx, `
		SELECT m.match_id, m.start_time, m.duration_s, m.game_mode, m.match_mode,
		       mp.hero_id, mp.team, m.winning_team = mp.team,
		       mp.kills, mp.deaths, mp.assists, mp.net_worth, mp.last_hits,
		       mp.player_damage, mp.player_healing, mp.level
		FROM match_players mp
		JOIN matches m ON m.match_id = mp.match_id
		WHERE mp.account_id = $1
		ORDER BY m.start_time, m.match_id`, accountID)
	if err != nil {
		return nil, err
	}
	var out []*v1.SeenMatch
	for rows.Next() {
		var (
			s     v1.SeenMatch
			start time.Time
		)
		if err := rows.Scan(&s.MatchId, &start, &s.DurationS, &s.GameMode, &s.MatchMode,
			&s.HeroId, &s.Team, &s.Won, &s.Kills, &s.Deaths, &s.Assists, &s.NetWorth, &s.LastHits,
			&s.PlayerDamage, &s.PlayerHealing, &s.Level); err != nil {
			rows.Close()
			return nil, err
		}
		s.StartTime = start.Unix()
		out = append(out, &s)
	}
	return out, rows.Err()
}
