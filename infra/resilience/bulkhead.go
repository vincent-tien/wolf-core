// bulkhead.go — Concurrency isolation via semaphore to prevent cascade failures.
package resilience

import (
	"context"
	"errors"
)

// ErrBulkheadFull is returned when all slots in the bulkhead are occupied and
// the caller's context is cancelled before a slot becomes available.
var ErrBulkheadFull = errors.New("bulkhead: all slots occupied")

// Bulkhead limits the number of concurrent operations using a semaphore
// pattern (buffered channel). This isolates failures by preventing any single
// dependency from consuming all available goroutines.
type Bulkhead struct {
	sem chan struct{}
}

// NewBulkhead creates a Bulkhead that permits at most maxConcurrent operations
// to execute simultaneously. maxConcurrent must be positive.
func NewBulkhead(maxConcurrent int) *Bulkhead {
	return &Bulkhead{
		sem: make(chan struct{}, maxConcurrent),
	}
}

// Execute runs fn once a slot is available. If ctx is cancelled before a slot
// opens, ErrBulkheadFull is returned without executing fn. The slot is released
// after fn returns regardless of outcome.
func (b *Bulkhead) Execute(ctx context.Context, fn func() error) error {
	select {
	case b.sem <- struct{}{}:
		defer func() { <-b.sem }()
		return fn()
	case <-ctx.Done():
		return ErrBulkheadFull
	}
}
