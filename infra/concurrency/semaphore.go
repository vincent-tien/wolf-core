// semaphore.go — Weighted semaphore limiter using x/sync/semaphore.
package concurrency

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// ConcurrencyLimiter restricts the number of concurrently executing operations
// using a weighted semaphore. It complements the channel-based
// resilience.Bulkhead with a semaphore-based alternative that supports
// context-aware blocking acquisition.
type ConcurrencyLimiter struct {
	sem *semaphore.Weighted
}

// NewConcurrencyLimiter creates a limiter that allows at most max concurrent
// executions. max is clamped to a minimum of 1.
func NewConcurrencyLimiter(max int) *ConcurrencyLimiter {
	if max <= 0 {
		max = 1
	}
	return &ConcurrencyLimiter{sem: semaphore.NewWeighted(int64(max))}
}

// Execute acquires one semaphore slot, runs fn, and releases the slot. If ctx
// is cancelled while waiting for a slot, Execute returns the context error
// without running fn.
func (l *ConcurrencyLimiter) Execute(ctx context.Context, fn func() error) error {
	if err := l.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer l.sem.Release(1)
	return fn()
}
