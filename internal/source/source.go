// Package source abstracts where match data comes from.
//
// Replay files themselves always come straight from Valve's CDN
// (replay<cluster>.valve.net). The only thing we can't derive ourselves yet is
// the per-match "salt" that forms the file URL, and the list of a player's
// matches. Those are behind small interfaces so the third-party provider
// (deadlock-api.com today) can be swapped for our own Steam GC client later.
package source

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Salts identifies replay artifacts for one match on Valve's CDN.
type Salts struct {
	MatchID      uint64 `json:"match_id"`
	ClusterID    uint32 `json:"cluster_id"`
	MetadataSalt uint32 `json:"metadata_salt"`
	ReplaySalt   uint32 `json:"replay_salt"`
}

// Deadlock's Steam app id, part of every replay URL.
const appID = 1422450

// MetadataURL is the CDN URL of the match metadata (~150 KB).
func (s Salts) MetadataURL() string {
	return fmt.Sprintf("http://replay%d.valve.net/%d/%d_%d.meta.bz2", s.ClusterID, appID, s.MatchID, s.MetadataSalt)
}

// DemoURL is the CDN URL of the full demo (~200 MB).
func (s Salts) DemoURL() string {
	return fmt.Sprintf("http://replay%d.valve.net/%d/%d_%d.dem.bz2", s.ClusterID, appID, s.MatchID, s.ReplaySalt)
}

// MatchSummary is one row of a player's match history.
type MatchSummary struct {
	MatchID        uint64 `json:"match_id"`
	HeroID         uint32 `json:"hero_id"`
	StartTime      int64  `json:"start_time"`
	DurationS      uint32 `json:"match_duration_s"`
	GameMode       int    `json:"game_mode"`
	MatchMode      int    `json:"match_mode"`
	Team           int    `json:"player_team"`
	Result         int    `json:"match_result"` // winning team
	Kills          uint32 `json:"player_kills"`
	Deaths         uint32 `json:"player_deaths"`
	Assists        uint32 `json:"player_assists"`
	NetWorth       uint32 `json:"net_worth"`
	LastHits       uint32 `json:"last_hits"`
	Denies         uint32 `json:"denies"`
	HeroLevel      uint32 `json:"hero_level"`
	AbandonedTimeS uint32 `json:"abandoned_time_s"`
}

// Won reports whether the player's team won.
func (m MatchSummary) Won() bool { return m.Team == m.Result }

// SaltProvider resolves CDN salts for a match.
type SaltProvider interface {
	Salts(ctx context.Context, matchID uint64) (Salts, error)
}

// HistoryProvider lists a player's matches, newest first.
type HistoryProvider interface {
	MatchHistory(ctx context.Context, accountID uint32) ([]MatchSummary, error)
}

// ProfileProvider returns Steam profiles of many accounts at once (at most
// MaxProfileBatch per call).
type ProfileProvider interface {
	SteamProfiles(ctx context.Context, ids []uint32) ([]SteamProfile, error)
}

// SteamProfile is a public Steam profile as deadlock-api caches it.
type SteamProfile struct {
	AccountID    uint32        `json:"account_id"`
	PersonaName  string        `json:"personaname"`
	ProfileURL   string        `json:"profileurl"`
	Avatar       string        `json:"avatar"`
	AvatarMedium string        `json:"avatarmedium"`
	AvatarFull   string        `json:"avatarfull"`
	CountryCode  *string       `json:"countrycode"`
	Friends      []SteamFriend `json:"friends"`
}

// SteamFriend is one entry of a profile's friend list.
type SteamFriend struct {
	AccountID   uint32     `json:"account_id"`
	FriendSince *time.Time `json:"friend_since"`
}

// MetaProvider downloads a match's .meta file as stored by a third party
// (compressed, the same bytes Valve's CDN serves).
type MetaProvider interface {
	MetadataRaw(ctx context.Context, matchID uint64) ([]byte, error)
}

// RateLimitError is a 429 from a provider: retry after RetryAfter, and
// don't count it as a failed attempt.
type RateLimitError struct {
	What       string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%s: rate limited, retry after %s", e.What, e.RetryAfter)
}

// ErrNotFound means the provider has no data for the request (e.g. salts for
// a match too old to still be hosted).
var ErrNotFound = errors.New("not found")

// steamID64Base is the SteamID64 of account id 0 (individual, public universe).
const steamID64Base = 76561197960265728

// SteamID64ToAccountID converts a 64-bit SteamID to the 32-bit account id
// used everywhere in Deadlock data.
func SteamID64ToAccountID(id uint64) uint32 {
	return uint32(id - steamID64Base)
}

// NormalizeAccountID accepts either a 32-bit account id or a SteamID64 and
// returns the account id. ok is false for zero and for values that are
// neither (above uint32 but outside the SteamID64 range).
func NormalizeAccountID(v uint64) (id uint32, ok bool) {
	switch {
	case v == 0:
		return 0, false
	case v <= math.MaxUint32:
		return uint32(v), true
	case v > steamID64Base && v-steamID64Base <= math.MaxUint32:
		return SteamID64ToAccountID(v), true
	default:
		return 0, false
	}
}

// ParseAccountID is NormalizeAccountID for text input.
func ParseAccountID(s string) (uint32, error) {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad account id %q", s)
	}
	id, ok := NormalizeAccountID(v)
	if !ok {
		return 0, fmt.Errorf("bad account id %q: neither an account id nor a SteamID64", s)
	}
	return id, nil
}
