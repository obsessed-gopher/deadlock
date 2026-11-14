package steamgc

import (
	"time"

	"github.com/obsessed-gopher/deadlock/internal/source"
)

// limits shape a token bucket of GC requests of one kind for one bot, with a
// reserve that only Fresh requests may dip into and an escalating pause after
// the GC says "rate limited". The bucket's state is a source.BotBudget kept
// in the Store; these functions only change it.
type limits struct {
	perHour float64 // refill rate
	burst   float64 // capacity
	reserve float64 // tokens kept for Fresh requests
}

// backoffs is the pause after the 1st, 2nd and further rate limits in a row.
var backoffs = []time.Duration{10 * time.Minute, 30 * time.Minute, time.Hour}

// full is the state of a bucket never used before.
func (l limits) full(now time.Time) source.BotBudget {
	return source.BotBudget{Tokens: l.burst, RefilledAt: now}
}

func (l limits) refill(s *source.BotBudget, now time.Time) {
	if now.After(s.RefilledAt) {
		s.Tokens = min(l.burst, s.Tokens+now.Sub(s.RefilledAt).Hours()*l.perHour)
		s.RefilledAt = now
	}
}

// take spends a token, or says how long until one is available: a Backlog
// request (fresh=false) must leave the reserve untouched.
func (l limits) take(s *source.BotBudget, now time.Time, fresh bool) (wait time.Duration, ok bool) {
	l.refill(s, now)
	if now.Before(s.PausedUntil) {
		return s.PausedUntil.Sub(now), false
	}
	floor := 0.0
	if !fresh {
		floor = l.reserve
	}
	if s.Tokens-1 < floor {
		missing := floor + 1 - s.Tokens
		return time.Duration(missing / l.perHour * float64(time.Hour)), false
	}
	s.Tokens--
	return 0, true
}

// limited records a "rate limited" answer: the bucket empties and pauses for
// longer with every one in a row. It returns the pause.
func limited(s *source.BotBudget, now time.Time) time.Duration {
	d := backoffs[min(s.Strikes, len(backoffs)-1)]
	s.Strikes++
	s.Tokens = 0
	s.RefilledAt = now
	s.PausedUntil = now.Add(d)
	return d
}

// succeeded resets the escalation.
func succeeded(s *source.BotBudget) { s.Strikes = 0 }
