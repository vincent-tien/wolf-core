// retry.go — Exponential backoff retry with jitter for transient failures.
//
// Used by the outbox worker, resilient HTTP client, and any infrastructure
// code that interacts with external services prone to transient errors.
// Jitter (±25%) prevents thundering herd when multiple instances retry
// against the same dependency simultaneously.
package resilience

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// WithRetry calls fn up to maxRetries+1 times (initial attempt plus retries)
// using exponential back-off with ±25 % jitter between attempts.
//
// The back-off duration after attempt i is:
//
//	baseDelay * 2^i * (1 + jitter)   where jitter ∈ [-0.25, +0.25)
//
// Context cancellation or deadline expiry aborts the retry loop immediately
// and returns ctx.Err() wrapped in a descriptive message.
//
// Parameters:
//
//	ctx        – governs the overall retry lifetime.
//	maxRetries – number of additional attempts after the first failure (0 = no retry).
//	baseDelay  – initial back-off duration before the first retry.
//	fn         – the operation to attempt; a nil error stops the loop.
func WithRetry(ctx context.Context, maxRetries int, baseDelay time.Duration, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check context before each attempt so that a cancelled context is
		// respected even when fn itself does not accept a context.
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("retry: context cancelled after %d attempt(s): %w", attempt, err)
		}

		if lastErr = fn(); lastErr == nil {
			return nil
		}

		// No delay after the last attempt.
		if attempt == maxRetries {
			break
		}

		delay := backoffDelay(baseDelay, attempt)
		timer := time.NewTimer(delay)

		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("retry: context cancelled waiting for back-off after %d attempt(s): %w", attempt+1, ctx.Err())
		case <-timer.C:
		}
	}

	return fmt.Errorf("retry: all %d attempt(s) failed: %w", maxRetries+1, lastErr)
}

// backoffDelay calculates the back-off duration for the given attempt index
// with ±25 % jitter to spread retries across instances.
func backoffDelay(base time.Duration, attempt int) time.Duration {
	// Exponential component: base * 2^attempt.
	exp := base
	for i := 0; i < attempt; i++ {
		exp *= 2
	}

	// Jitter: a random multiplier in [0.75, 1.25).
	//nolint:gosec // rand is intentional here; crypto randomness is unnecessary for jitter.
	jitter := 0.75 + rand.Float64()*0.5
	return time.Duration(float64(exp) * jitter)
}
