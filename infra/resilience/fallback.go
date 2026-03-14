// fallback.go — Generic fallback combinator for degraded-mode operation.
package resilience

import "context"

// WithFallback executes primary and returns its result on success. If primary
// returns a non-nil error, fallback is invoked and its result is returned
// instead. Both functions receive the same context, so cancellation propagates
// to whichever function is active.
func WithFallback[T any](ctx context.Context, primary, fallback func(ctx context.Context) (T, error)) (T, error) {
	result, err := primary(ctx)
	if err == nil {
		return result, nil
	}
	return fallback(ctx)
}
