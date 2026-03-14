package transport_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/transport"
)

type stubSender struct {
	err   error
	calls atomic.Int64
}

func (s *stubSender) Send(_ context.Context, _ messenger.Envelope) error {
	s.calls.Add(1)
	return s.err
}

func TestCircuitSender_ClosedState_SendSucceeds(t *testing.T) {
	inner := &stubSender{}
	cs := transport.NewCircuitSender("test", inner)

	env := messenger.NewEnvelope("msg")
	if err := cs.Send(context.Background(), env); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", inner.calls.Load())
	}
}

func TestCircuitSender_OpensAfterConsecutiveFailures(t *testing.T) {
	inner := &stubSender{err: errors.New("down")}
	cs := transport.NewCircuitSender("test", inner,
		transport.WithBreakerSettings(gobreaker.Settings{
			Name: "test",
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= 3
			},
			Timeout: time.Second,
		}),
	)

	env := messenger.NewEnvelope("msg")

	// 3 failures to trip the breaker.
	for range 3 {
		_ = cs.Send(context.Background(), env)
	}

	// Next call should get ErrCircuitOpen.
	err := cs.Send(context.Background(), env)
	if !errors.Is(err, messenger.ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
}

func TestCircuitSender_OpenState_ReturnsFallback(t *testing.T) {
	inner := &stubSender{err: errors.New("down")}
	fallback := &stubSender{}

	cs := transport.NewCircuitSender("test", inner,
		transport.WithBreakerSettings(gobreaker.Settings{
			Name: "test",
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= 2
			},
			Timeout: 5 * time.Second,
		}),
		transport.WithFallback(fallback),
	)

	env := messenger.NewEnvelope("msg")

	// Trip the breaker.
	for range 2 {
		_ = cs.Send(context.Background(), env)
	}

	// Should route to fallback.
	if err := cs.Send(context.Background(), env); err != nil {
		t.Fatalf("fallback send: %v", err)
	}
	if fallback.calls.Load() != 1 {
		t.Errorf("fallback calls = %d, want 1", fallback.calls.Load())
	}
}

func TestCircuitSender_HalfOpenProbeSuccess_ClosesCircuit(t *testing.T) {
	failErr := errors.New("down")
	inner := &stubSender{err: failErr}

	cs := transport.NewCircuitSender("test", inner,
		transport.WithBreakerSettings(gobreaker.Settings{
			Name:        "test",
			MaxRequests: 1,
			Timeout:     50 * time.Millisecond, // fast transition to half-open
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= 2
			},
		}),
	)

	env := messenger.NewEnvelope("msg")

	// Trip the breaker.
	for range 2 {
		_ = cs.Send(context.Background(), env)
	}

	// Wait for half-open.
	time.Sleep(100 * time.Millisecond)

	// Fix the inner sender.
	inner.err = nil

	// Half-open probe should succeed → circuit closes.
	if err := cs.Send(context.Background(), env); err != nil {
		t.Fatalf("half-open probe: %v", err)
	}

	// Verify circuit is closed — further sends succeed.
	if err := cs.Send(context.Background(), env); err != nil {
		t.Fatalf("after close: %v", err)
	}
}

func TestCircuitSender_State(t *testing.T) {
	inner := &stubSender{}
	cs := transport.NewCircuitSender("test", inner)

	if got := cs.State(); got != gobreaker.StateClosed {
		t.Errorf("State() = %v, want StateClosed", got)
	}
}

func BenchmarkCircuitSender_HappyPath(b *testing.B) {
	inner := &stubSender{}
	cs := transport.NewCircuitSender("bench", inner)
	env := messenger.NewEnvelope("msg")
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		_ = cs.Send(ctx, env)
	}
}
