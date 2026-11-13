package source

import (
	"cmp"
	"context"
	"errors"
	"slices"
)

// SaltChain asks providers in order and returns the first salts found.
//
// When all of them fail, the error says what to do next: ErrNotFound only if
// every provider has no salts (the match is gone), a *RateLimitError if any
// was rate limited (try later), otherwise the other failures.
type SaltChain []SaltProvider

// Salts implements SaltProvider.
func (c SaltChain) Salts(ctx context.Context, matchID uint64) (Salts, error) {
	var notFound, other []error
	var limited *RateLimitError
	for _, p := range c {
		s, err := p.Salts(ctx, matchID)
		if err == nil {
			return s, nil
		}
		if rl, ok := errors.AsType[*RateLimitError](err); ok && limited == nil {
			limited = rl
		}
		if errors.Is(err, ErrNotFound) {
			notFound = append(notFound, err)
		} else {
			other = append(other, err)
		}
	}
	switch {
	case len(other) == 0:
		return Salts{}, errors.Join(append(notFound, ErrNotFound)...)
	case limited != nil:
		return Salts{}, limited
	default:
		return Salts{}, errors.Join(other...)
	}
}

// MergedHistory joins the histories of several providers: a fresh but short
// one (the GC's newest page) and a full but late one (deadlock-api). Entries
// of earlier providers win; the result is newest first. It fails only when
// every provider fails; OnError, if set, hears about each failure.
type MergedHistory struct {
	Providers []HistoryProvider
	OnError   func(provider int, err error)
}

// MatchHistory implements HistoryProvider.
func (h MergedHistory) MatchHistory(ctx context.Context, accountID uint32) ([]MatchSummary, error) {
	var out []MatchSummary
	seen := map[uint64]bool{}
	var errs []error
	for i, p := range h.Providers {
		ms, err := p.MatchHistory(ctx, accountID)
		if err != nil {
			errs = append(errs, err)
			if h.OnError != nil {
				h.OnError(i, err)
			}
			continue
		}
		for _, m := range ms {
			if !seen[m.MatchID] {
				seen[m.MatchID] = true
				out = append(out, m)
			}
		}
	}
	if len(errs) == len(h.Providers) {
		return nil, errors.Join(errs...)
	}
	slices.SortStableFunc(out, func(a, b MatchSummary) int {
		return cmp.Or(cmp.Compare(b.StartTime, a.StartTime), cmp.Compare(b.MatchID, a.MatchID))
	})
	return out, nil
}
