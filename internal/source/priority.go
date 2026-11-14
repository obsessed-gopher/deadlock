package source

import (
	"context"
	"time"
)

// QueuedMatch is a match waiting for its metadata.
type QueuedMatch struct {
	MatchID   uint64
	StartTime time.Time
}

// FreshAge is how old a match may be and still count as Fresh: deadlock-api
// usually has the metadata of older ones, the GC budget goes to the new.
const FreshAge = 48 * time.Hour

// PriorityFor is the priority of downloading a match that started at start.
func PriorityFor(start, now time.Time) Priority {
	if now.Sub(start) > FreshAge {
		return Backlog
	}
	return Fresh
}

// Priority tells a provider with a scarce budget (the Steam GC) how urgent a
// request is: a match that just ended goes before the backlog of old ones.
type Priority int

const (
	// Fresh is the default: recent matches and anything a person waits for.
	Fresh Priority = iota
	// Backlog is old matches that can wait for spare budget.
	Backlog
)

type priorityKey struct{}

// WithPriority marks the requests made with ctx.
func WithPriority(ctx context.Context, p Priority) context.Context {
	return context.WithValue(ctx, priorityKey{}, p)
}

// PriorityOf is the priority ctx was marked with, Fresh if none.
func PriorityOf(ctx context.Context) Priority {
	p, _ := ctx.Value(priorityKey{}).(Priority)
	return p
}
