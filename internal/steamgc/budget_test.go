package steamgc

import (
	"testing"
	"time"
)

func TestBucketReserve(t *testing.T) {
	t0 := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	l := limits{40, 40, 20}
	s := l.full(t0)

	// The backlog spends only what is above the reserve.
	for i := range 20 {
		if _, ok := l.take(&s, t0, false); !ok {
			t.Fatalf("backlog request %d refused with %v tokens", i+1, s.Tokens)
		}
	}
	wait, ok := l.take(&s, t0, false)
	if ok || wait != 90*time.Second {
		t.Errorf("backlog at the reserve: ok=%v wait=%v; want a wait of one token (90s at 40/h)", ok, wait)
	}
	// Fresh requests dip into the reserve.
	for i := range 20 {
		if _, ok := l.take(&s, t0, true); !ok {
			t.Fatalf("fresh request %d refused", i+1)
		}
	}
	if wait, ok := l.take(&s, t0, true); ok || wait != 90*time.Second {
		t.Errorf("empty bucket: ok=%v wait=%v; want 90s", ok, wait)
	}
	// It refills at the rate, up to the burst.
	if _, ok := l.take(&s, t0.Add(90*time.Second), true); !ok {
		t.Error("a token after 90s: refused")
	}
	l.refill(&s, t0.Add(10*time.Hour))
	if s.Tokens != 40 {
		t.Errorf("tokens after a long idle = %v, want the burst 40", s.Tokens)
	}
}

func TestBucketBackoff(t *testing.T) {
	t0 := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	l := limits{40, 40, 20}
	s := l.full(t0)
	now := t0
	for i, want := range []time.Duration{10 * time.Minute, 30 * time.Minute, time.Hour, time.Hour} {
		if got := limited(&s, now); got != want {
			t.Errorf("rate limit %d in a row: pause %v, want %v", i+1, got, want)
		}
		if wait, ok := l.take(&s, now.Add(time.Minute), true); ok || wait != want-time.Minute {
			t.Errorf("during the pause: ok=%v wait=%v; want %v", ok, wait, want-time.Minute)
		}
		now = now.Add(want)
	}
	// After the pause the bucket refills from empty, and a success resets the escalation.
	if _, ok := l.take(&s, now.Add(2*time.Minute), true); !ok {
		t.Error("after the pause: refused")
	}
	succeeded(&s)
	if got := limited(&s, now); got != 10*time.Minute {
		t.Errorf("after a success: pause %v, want 10m again", got)
	}
}
