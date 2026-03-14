// retry.go — Messenger middleware that retries failed dispatch with exponential backoff.
package middleware

import (
	"context"
	"math"
	"math/rand/v2"
	"time"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxRetries int
	Delay      time.Duration
	Multiplier float64
	MaxDelay   time.Duration
	Jitter     float64 // 0.0 to 1.0
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		Delay:      100 * time.Millisecond,
		Multiplier: 2.0,
		MaxDelay:   10 * time.Second,
		Jitter:     0.1,
	}
}

// Retry retries failed dispatch with exponential backoff.
// Intended for the CONSUME side (worker), not dispatch side.
type Retry struct {
	cfg RetryConfig
}

// NewRetry creates a retry middleware.
func NewRetry(cfg RetryConfig) *Retry {
	return &Retry{cfg: cfg}
}

func (m *Retry) Handle(ctx context.Context, env messenger.Envelope, next messenger.MiddlewareNext) (messenger.DispatchResult, error) {
	result, err := next(ctx, env)
	if err == nil {
		return result, nil
	}

	lastErr := err
	for attempt := 1; attempt <= m.cfg.MaxRetries; attempt++ {
		if sleepErr := sleepWithContext(ctx, m.backoff(attempt)); sleepErr != nil {
			return messenger.DispatchResult{}, sleepErr
		}

		env = env.WithStamp(stamp.RedeliveryStamp{
			RetryCount: attempt,
			LastError:  lastErr.Error(),
		})
		result, lastErr = next(ctx, env)
		if lastErr == nil {
			return result, nil
		}
	}

	return messenger.DispatchResult{}, lastErr
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (m *Retry) backoff(attempt int) time.Duration {
	d := float64(m.cfg.Delay) * math.Pow(m.cfg.Multiplier, float64(attempt-1))
	if m.cfg.Jitter > 0 {
		jitter := d * m.cfg.Jitter * (rand.Float64()*2 - 1)
		d += jitter
	}
	if d > float64(m.cfg.MaxDelay) {
		d = float64(m.cfg.MaxDelay)
	}
	return time.Duration(d)
}
