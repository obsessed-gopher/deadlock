// Package steamgc talks to Deadlock's Steam Game Coordinator directly, as a
// logged-in bot account: a player's match history and the CDN salts of a
// match, without a third-party API in between.
//
// It wraps github.com/paralin/go-steam (the Steam connection and modern
// authentication) and github.com/paralin/go-deadlock (the GC protocol); both
// stay behind this package so the rest of the code sees only source types.
package steamgc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	deadlock "github.com/paralin/go-deadlock"
	devents "github.com/paralin/go-deadlock/events"
	gc "github.com/paralin/go-deadlock/protocol"
	steam "github.com/paralin/go-steam"
	"github.com/paralin/go-steam/protocol/steamlang"
	"github.com/paralin/go-steam/steamid"
	"github.com/sirupsen/logrus"

	"github.com/obsessed-gopher/deadlock/internal/source"
)

// ErrNotReady is returned while there is no GC session.
var ErrNotReady = errors.New("steam gc: no session")

// Client is one bot session with the Deadlock GC.
type Client struct {
	log      *slog.Logger
	username string
	token    string

	sc *steam.Client
	dl *deadlock.Deadlock

	stop context.CancelFunc // ends the session context

	mu      sync.Mutex
	welcome chan struct{} // closed once the GC admits the session
	done    chan struct{} // closed when the event loop exits
	err     error         // why the session ended
}

// Dial logs the bot on with a refresh token from Login and waits until the
// GC admits the session or ctx ends.
func Dial(ctx context.Context, log *slog.Logger, username, token string) (*Client, error) {
	if username == "" || token == "" {
		return nil, errors.New("steam gc: username and token are required")
	}
	c := &Client{
		log:      log,
		username: username,
		token:    token,
		sc:       steam.NewClient(),
		welcome:  make(chan struct{}),
		done:     make(chan struct{}),
	}
	c.dl = deadlock.New(c.sc, quietLogrus())
	// The session outlives ctx, which only bounds the wait for the GC:
	// go-steam ties the connection to the context it was dialed with.
	sess, stop := context.WithCancel(context.WithoutCancel(ctx))
	c.stop = stop
	connected := make(chan error, 1)
	go func() { _, err := c.sc.ConnectContext(sess); connected <- err }()
	select {
	case err := <-connected:
		if err != nil {
			stop()
			return nil, fmt.Errorf("steam gc: connect: %w", err)
		}
	case <-ctx.Done():
		stop()
		return nil, fmt.Errorf("steam gc: connect: %w", ctx.Err())
	}
	go c.loop(sess)
	select {
	case <-c.welcome:
		return c, nil
	case <-c.done:
		return nil, c.cause()
	case <-ctx.Done():
		c.Close()
		return nil, fmt.Errorf("steam gc: waiting for the GC: %w", ctx.Err())
	}
}

// loop consumes Steam and GC events; go-steam requires a steady reader.
func (c *Client) loop(ctx context.Context) {
	defer close(c.done)
	welcomed := false
	for {
		var ev any
		select {
		case <-ctx.Done():
			return
		case ev = <-c.sc.Events():
		}
		c.log.Debug("steam event", "type", fmt.Sprintf("%T", ev))
		switch e := ev.(type) {
		case *steam.ConnectedEvent:
			lctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := c.sc.Auth.LogOn(lctx, &steam.LogOnDetails{Username: c.username, AccessToken: c.token})
			cancel()
			if err != nil {
				c.fail(fmt.Errorf("steam gc: log on: %w", err))
				return
			}
		case *steam.LoggedOnEvent:
			c.log.Info("steam bot logged on", "steam_id", c.sc.SteamId().ToUint64())
			c.dl.SetPlaying(true)
			go c.hello()
		case *steam.FriendStateEvent:
			// Players add the bot to let it read their match history.
			if e.Relationship == steamlang.EFriendRelationship_RequestRecipient {
				c.sc.Social.AddFriend(e.SteamId)
				c.log.Info("steam friend request accepted", "account", e.SteamId.GetAccountId())
			}
		case *devents.GCConnectionStatusChanged:
			c.log.Info("deadlock gc status", "old", e.OldState.String(), "new", e.NewState.String())
		case *devents.UnhandledGCPacket:
			c.log.Debug("unhandled gc packet", "msg", e.Packet.MsgType)
		case *devents.ClientWelcomed:
			if !welcomed {
				welcomed = true
				close(c.welcome)
			}
			c.log.Info("deadlock gc session ready")
		case *steam.LogOnFailedEvent:
			c.fail(fmt.Errorf("steam gc: log on failed: %v", e.Result))
			return
		case *steam.LoggedOffEvent:
			c.fail(fmt.Errorf("steam gc: logged off: %v", e.Result))
			return
		case *steam.DisconnectedEvent:
			c.fail(errors.New("steam gc: disconnected"))
			return
		case steam.FatalErrorEvent:
			c.fail(fmt.Errorf("steam gc: %w", e))
			return
		}
	}
}

// hello repeats the GC hello until the GC welcomes the session: the GC
// ignores hellos that come before it learns the bot started the game.
func (c *Client) hello() {
	t := time.NewTicker(helloEvery)
	defer t.Stop()
	for {
		c.dl.SayHello()
		select {
		case <-c.welcome:
			return
		case <-c.done:
			return
		case <-t.C:
			c.log.Debug("deadlock gc hello again")
		}
	}
}

const helloEvery = 5 * time.Second

func (c *Client) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.mu.Unlock()
	if c.stop != nil {
		c.stop()
	}
	c.dl.Close()
	c.sc.Disconnect()
}

func (c *Client) cause() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		return ErrNotReady
	}
	return c.err
}

// Done is closed when the session ends; Err then tells why.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err is why the session ended, nil while it is alive.
func (c *Client) Err() error {
	select {
	case <-c.done:
		return c.cause()
	default:
		return nil
	}
}

// Close ends the session.
func (c *Client) Close() {
	c.fail(errors.New("steam gc: closed"))
}

// IsFriend reports whether the account is the bot's Steam friend.
func (c *Client) IsFriend(accountID uint32) bool {
	f, err := c.sc.Social.Friends.ById(steamid.NewIdAdv(accountID, 1, int32(steamlang.EUniverse_Public), steamlang.EAccountType_Individual))
	return err == nil && f.Relationship == steamlang.EFriendRelationship_Friend
}

// MatchHistory is the newest page of a player's match history, newest first.
// The GC shows other players' history only to their Steam friends.
func (c *Client) MatchHistory(ctx context.Context, accountID uint32) ([]source.MatchSummary, error) {
	resp, err := c.dl.GetMatchHistory(ctx, accountID, 0,
		gc.ECitadelGameMode_k_ECitadelGameMode_Invalid,
		gc.ECitadelMatchMode_k_ECitadelMatchMode_Invalid,
		gc.ECitadelRankedType_k_eCitadelRankedType_Invalid, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("steam gc: match history of %d: %w", accountID, err)
	}
	if err := historyErr(resp.GetResult()); err != nil {
		return nil, fmt.Errorf("steam gc: match history of %d: %w", accountID, err)
	}
	out := make([]source.MatchSummary, 0, len(resp.GetMatches()))
	for _, m := range resp.GetMatches() {
		out = append(out, source.MatchSummary{
			MatchID:        m.GetMatchId(),
			HeroID:         m.GetHeroId(),
			StartTime:      int64(m.GetStartTime()),
			DurationS:      m.GetMatchDurationS(),
			GameMode:       int(m.GetGameMode()),
			MatchMode:      int(m.GetMatchMode()),
			Team:           int(m.GetPlayerTeam()),
			Result:         int(m.GetMatchResult()),
			Kills:          m.GetPlayerKills(),
			Deaths:         m.GetPlayerDeaths(),
			Assists:        m.GetPlayerAssists(),
			NetWorth:       m.GetNetWorth(),
			LastHits:       m.GetLastHits(),
			Denies:         m.GetDenies(),
			HeroLevel:      m.GetHeroLevel(),
			AbandonedTimeS: m.GetAbandonedTimeS(),
		})
	}
	return out, nil
}

// Salts asks the GC for the replay salts of a match. The replay group is the
// CDN cluster of replay<cluster>.valve.net.
func (c *Client) Salts(ctx context.Context, matchID uint64) (source.Salts, error) {
	resp, err := c.dl.GetMatchMetaData(ctx, matchID, 0, 0)
	if err != nil {
		return source.Salts{}, fmt.Errorf("steam gc: salts of %d: %w", matchID, err)
	}
	if err := saltsErr(resp.GetResult()); err != nil {
		return source.Salts{}, fmt.Errorf("steam gc: salts of %d: %w", matchID, err)
	}
	return source.Salts{
		MatchID:      matchID,
		ClusterID:    resp.GetReplayGroupId(),
		MetadataSalt: resp.GetMetadataSalt(),
		ReplaySalt:   resp.GetReplaySalt(),
	}, nil
}

// RetryBusy is how long to leave the GC alone after it says it is busy or
// rate limits us.
const RetryBusy = 10 * time.Minute

// historyErr maps a match history result to the source error conventions.
func historyErr(r gc.CMsgClientToGCGetMatchHistoryResponse_EResult) error {
	switch r {
	case gc.CMsgClientToGCGetMatchHistoryResponse_k_eResult_Success:
		return nil
	case gc.CMsgClientToGCGetMatchHistoryResponse_k_eResult_RateLimited,
		gc.CMsgClientToGCGetMatchHistoryResponse_k_eResult_TooBusy,
		gc.CMsgClientToGCGetMatchHistoryResponse_k_eResult_TemporarilyDisabled:
		return &source.RateLimitError{What: "steam gc history: " + r.String(), RetryAfter: RetryBusy}
	default:
		return errors.New(r.String())
	}
}

// saltsErr maps a match metadata result to the source error conventions: an
// unknown match is ErrNotFound, a busy GC or a match still being played is
// "try later".
func saltsErr(r gc.CMsgClientToGCGetMatchMetaDataResponse_EResult) error {
	switch r {
	case gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_Success:
		return nil
	case gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_InvalidMatch:
		return fmt.Errorf("%s: %w", r.String(), source.ErrNotFound)
	case gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_RateLimited,
		gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_TooBusy,
		gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_TemporarilyDisabled,
		gc.CMsgClientToGCGetMatchMetaDataResponse_k_eResult_MatchInFlight:
		return &source.RateLimitError{What: "steam gc salts: " + r.String(), RetryAfter: RetryBusy}
	default:
		return errors.New(r.String())
	}
}

// quietLogrus silences go-deadlock's logrus output; errors reach us as
// events and returned errors.
func quietLogrus() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(io.Discard)
	return l
}
