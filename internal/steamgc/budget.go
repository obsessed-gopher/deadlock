package steamgc

import "time"

// bucket is a token bucket of GC requests of one kind for one bot, with a
// reserve that only Fresh requests may dip into and an escalating pause after
// the GC says "rate limited". Not safe for concurrent use: the bot guards it.
type bucket struct {
	perHour float64 // refill rate
	burst   float64 // capacity
	reserve float64 // tokens kept for Fresh requests

	tokens      float64
	at          time.Time // last refill
	pausedUntil time.Time
	strikes     int // rate limits in a row
}

// backoffs is the pause after the 1st, 2nd and further rate limits in a row.
var backoffs = []time.Duration{10 * time.Minute, 30 * time.Minute, time.Hour}

func newBucket(perHour, burst, reserve float64, now time.Time) *bucket {
	return &bucket{perHour: perHour, burst: burst, reserve: reserve, tokens: burst, at: now}
}

func (b *bucket) refill(now time.Time) {
	if now.After(b.at) {
		b.tokens = min(b.burst, b.tokens+now.Sub(b.at).Hours()*b.perHour)
		b.at = now
	}
}

// take spends a token, or says how long until one is available: a Backlog
// request (fresh=false) must leave the reserve untouched.
func (b *bucket) take(now time.Time, fresh bool) (wait time.Duration, ok bool) {
	b.refill(now)
	if now.Before(b.pausedUntil) {
		return b.pausedUntil.Sub(now), false
	}
	floor := 0.0
	if !fresh {
		floor = b.reserve
	}
	if b.tokens-1 < floor {
		missing := floor + 1 - b.tokens
		return time.Duration(missing / b.perHour * float64(time.Hour)), false
	}
	b.tokens--
	return 0, true
}

// limited records a "rate limited" answer: the bucket empties and pauses for
// longer with every one in a row. It returns the pause.
func (b *bucket) limited(now time.Time) time.Duration {
	d := backoffs[min(b.strikes, len(backoffs)-1)]
	b.strikes++
	b.tokens = 0
	b.at = now
	b.pausedUntil = now.Add(d)
	return d
}

// succeeded resets the escalation.
func (b *bucket) succeeded() { b.strikes = 0 }
