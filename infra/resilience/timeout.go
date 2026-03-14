// timeout.go — Generic deadline wrapper for context-aware operations.
package resilience

import (
	"context"
	"fmt"
	"time"
)

// WithTimeout executes fn within a context constrained by the given duration.
// If fn completes before the deadline, its result is returned directly. If the
// deadline is reached first, WithTimeout returns a context.DeadlineExceeded
// error. The derived context is always cancelled on return to free resources.
func WithTimeout[T any](ctx context.Context, duration time.Duration, fn func(ctx context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	result, err := fn(ctx)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("timeout: %w", err)
	}
	return result, nil
}
