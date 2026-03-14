// circuit.go — Circuit breaker decorator for transport senders.
package transport

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker"
	"github.com/vincent-tien/wolf-core/messenger"
)

// CircuitOption configures a CircuitSender.
type CircuitOption func(*CircuitSender)

// WithFallback sets a fallback sender used when the circuit is open.
func WithFallback(s Sender) CircuitOption {
	return func(cs *CircuitSender) { cs.fallback = s }
}

// WithBreakerSettings overrides the default gobreaker settings.
func WithBreakerSettings(s gobreaker.Settings) CircuitOption {
	return func(cs *CircuitSender) { cs.settings = &s }
}

// CircuitSender wraps a Sender with a circuit breaker.
// When the inner sender fails consecutively, the breaker opens and sends are
// rejected immediately — preventing cascading failure to downstream transports.
type CircuitSender struct {
	inner    Sender
	breaker  *gobreaker.CircuitBreaker
	fallback Sender
	settings *gobreaker.Settings
}

// NewCircuitSender decorates inner with a per-transport circuit breaker.
//
// Default thresholds: 5 consecutive failures to trip, 30s open timeout,
// 3 half-open probe requests, 60s counting interval.
//
// Uses consecutive-failure strategy (not error-rate) because async transports
// exhibit bursty failure patterns — a few intermittent errors should not trip
// the breaker, but sustained outage (5 in a row) should.
func NewCircuitSender(name string, inner Sender, opts ...CircuitOption) *CircuitSender {
	cs := &CircuitSender{inner: inner}
	for _, opt := range opts {
		opt(cs)
	}

	var settings gobreaker.Settings
	if cs.settings != nil {
		settings = *cs.settings
	} else {
		settings = gobreaker.Settings{
			Name:        "messenger.transport." + name,
			MaxRequests: 3,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= 5
			},
		}
	}

	cs.breaker = gobreaker.NewCircuitBreaker(settings)
	return cs
}

func (cs *CircuitSender) Send(ctx context.Context, env messenger.Envelope) error {
	// gobreaker.Execute requires func() (any, error) — closure alloc is unavoidable.
	_, err := cs.breaker.Execute(func() (any, error) {
		return nil, cs.inner.Send(ctx, env)
	})
	if err == nil {
		return nil
	}

	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		if cs.fallback != nil {
			return cs.fallback.Send(ctx, env)
		}
		return fmt.Errorf("%w: transport %q", messenger.ErrCircuitOpen, cs.breaker.Name())
	}

	return fmt.Errorf("circuit sender %q: %w", cs.breaker.Name(), err)
}

// State returns the current circuit breaker state (for observability).
func (cs *CircuitSender) State() gobreaker.State {
	return cs.breaker.State()
}
