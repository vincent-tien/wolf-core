package middleware_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/middleware"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

type testCmd struct{ ID string }

func (testCmd) MessageName() string { return "test.Cmd" }

var testLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

func passThrough(_ context.Context, env messenger.Envelope) (messenger.DispatchResult, error) {
	return messenger.DispatchResult{Envelope: env}, nil
}

func failNext(_ context.Context, _ messenger.Envelope) (messenger.DispatchResult, error) {
	return messenger.DispatchResult{}, errors.New("handler failed")
}

// ── Recovery ──

func TestRecovery_NoPanic(t *testing.T) {
	mw := middleware.NewRecovery(testLogger)
	env := messenger.NewEnvelope(testCmd{ID: "1"})
	_, err := mw.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRecovery_CatchesPanic(t *testing.T) {
	mw := middleware.NewRecovery(testLogger)
	env := messenger.NewEnvelope(testCmd{ID: "2"})

	panicNext := func(_ context.Context, _ messenger.Envelope) (messenger.DispatchResult, error) {
		panic("boom")
	}

	_, err := mw.Handle(context.Background(), env, panicNext)
	if err == nil {
		t.Fatal("expected error from recovered panic")
	}
	if got := err.Error(); !strings.Contains(got, "panic") {
		t.Errorf("error should mention panic: %q", got)
	}
}

// ── Logging ──

func TestLogging_PassesThrough(t *testing.T) {
	mw := middleware.NewLogging(testLogger)
	env := messenger.NewEnvelope(testCmd{ID: "3"})
	result, err := mw.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Async {
		t.Error("expected sync result")
	}
}

func TestLogging_PropagatesError(t *testing.T) {
	mw := middleware.NewLogging(testLogger)
	env := messenger.NewEnvelope(testCmd{ID: "4"})
	_, err := mw.Handle(context.Background(), env, failNext)
	if err == nil {
		t.Error("expected error propagation")
	}
}

// ── Validation ──

func TestValidation_PassesValid(t *testing.T) {
	mw := middleware.NewValidation(func(_ any) error { return nil })
	env := messenger.NewEnvelope(testCmd{ID: "5"})
	_, err := mw.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidation_BlocksInvalid(t *testing.T) {
	mw := middleware.NewValidation(func(_ any) error {
		return errors.New("invalid")
	})
	env := messenger.NewEnvelope(testCmd{ID: "6"})

	var nextCalled bool
	_, err := mw.Handle(context.Background(), env, func(_ context.Context, _ messenger.Envelope) (messenger.DispatchResult, error) {
		nextCalled = true
		return messenger.DispatchResult{}, nil
	})
	if err == nil {
		t.Error("expected validation error")
	}
	if nextCalled {
		t.Error("next should not be called on validation failure")
	}
}

// ── Timeout ──

func TestTimeout_PassesWithinLimit(t *testing.T) {
	mw := middleware.NewTimeout(time.Second)
	env := messenger.NewEnvelope(testCmd{ID: "7"})
	_, err := mw.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTimeout_ExceedsLimit(t *testing.T) {
	mw := middleware.NewTimeout(10 * time.Millisecond)
	env := messenger.NewEnvelope(testCmd{ID: "8"})

	slowNext := func(ctx context.Context, env messenger.Envelope) (messenger.DispatchResult, error) {
		select {
		case <-ctx.Done():
			return messenger.DispatchResult{}, ctx.Err()
		case <-time.After(time.Second):
			return messenger.DispatchResult{Envelope: env}, nil
		}
	}

	_, err := mw.Handle(context.Background(), env, slowNext)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestTimeout_PerTypeOverride(t *testing.T) {
	mw := middleware.NewTimeout(time.Second).
		WithTypeTimeout("test.Cmd", 10*time.Millisecond)
	env := messenger.NewEnvelope(testCmd{ID: "9"})

	slowNext := func(ctx context.Context, _ messenger.Envelope) (messenger.DispatchResult, error) {
		select {
		case <-ctx.Done():
			return messenger.DispatchResult{}, ctx.Err()
		case <-time.After(time.Second):
			return messenger.DispatchResult{}, nil
		}
	}

	_, err := mw.Handle(context.Background(), env, slowNext)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("per-type timeout: err = %v, want DeadlineExceeded", err)
	}
}

// ── Retry ──

func TestRetry_NoRetryOnSuccess(t *testing.T) {
	cfg := middleware.DefaultRetryConfig()
	mw := middleware.NewRetry(cfg)
	env := messenger.NewEnvelope(testCmd{ID: "10"})

	calls := 0
	_, err := mw.Handle(context.Background(), env, func(_ context.Context, _ messenger.Envelope) (messenger.DispatchResult, error) {
		calls++
		return messenger.DispatchResult{}, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetry_RetriesOnFailure(t *testing.T) {
	cfg := middleware.RetryConfig{
		MaxRetries: 3,
		Delay:      time.Millisecond,
		Multiplier: 1.0,
		MaxDelay:   10 * time.Millisecond,
	}
	mw := middleware.NewRetry(cfg)
	env := messenger.NewEnvelope(testCmd{ID: "11"})

	calls := 0
	_, err := mw.Handle(context.Background(), env, func(_ context.Context, env messenger.Envelope) (messenger.DispatchResult, error) {
		calls++
		if calls < 3 {
			return messenger.DispatchResult{}, errors.New("transient")
		}
		return messenger.DispatchResult{Envelope: env}, nil
	})
	if err != nil {
		t.Errorf("unexpected error after retries: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetry_ExhaustsRetries(t *testing.T) {
	cfg := middleware.RetryConfig{
		MaxRetries: 2,
		Delay:      time.Millisecond,
		Multiplier: 1.0,
		MaxDelay:   10 * time.Millisecond,
	}
	mw := middleware.NewRetry(cfg)
	env := messenger.NewEnvelope(testCmd{ID: "12"})

	_, err := mw.Handle(context.Background(), env, failNext)
	if err == nil {
		t.Error("expected error after exhausting retries")
	}
}

func TestRetry_AddsRedeliveryStamp(t *testing.T) {
	cfg := middleware.RetryConfig{
		MaxRetries: 2,
		Delay:      time.Millisecond,
		Multiplier: 1.0,
		MaxDelay:   10 * time.Millisecond,
	}
	mw := middleware.NewRetry(cfg)
	env := messenger.NewEnvelope(testCmd{ID: "13"})

	calls := 0
	_, _ = mw.Handle(context.Background(), env, func(_ context.Context, env messenger.Envelope) (messenger.DispatchResult, error) {
		calls++
		if calls <= 2 {
			if calls == 2 {
				if !env.HasStamp(stamp.NameRedelivery) {
					t.Error("expected RedeliveryStamp on retry")
				}
			}
			return messenger.DispatchResult{}, errors.New("fail")
		}
		return messenger.DispatchResult{Envelope: env}, nil
	})
}

func TestRetry_RespectsContextCancellation(t *testing.T) {
	cfg := middleware.RetryConfig{
		MaxRetries: 10,
		Delay:      100 * time.Millisecond,
		Multiplier: 1.0,
		MaxDelay:   time.Second,
	}
	mw := middleware.NewRetry(cfg)
	env := messenger.NewEnvelope(testCmd{ID: "14"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := mw.Handle(ctx, env, failNext)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
}

// ── Tracing (structural test — no real OTel collector) ──

func TestTracing_PassesThrough(t *testing.T) {
	mw := middleware.NewTracing("test-tracer")
	env := messenger.NewEnvelope(testCmd{ID: "15"})
	_, err := mw.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTracing_PropagatesError(t *testing.T) {
	mw := middleware.NewTracing("test-tracer")
	env := messenger.NewEnvelope(testCmd{ID: "16"})
	_, err := mw.Handle(context.Background(), env, failNext)
	if err == nil {
		t.Error("expected error propagation")
	}
}

