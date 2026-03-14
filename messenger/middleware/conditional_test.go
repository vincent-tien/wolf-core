package middleware_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/middleware"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

type testQuery struct{ ID string }

func (testQuery) MessageName() string { return "test.Query" }

// trackingMiddleware records whether Handle was called.
type trackingMiddleware struct {
	called atomic.Bool
}

func (m *trackingMiddleware) Handle(ctx context.Context, env messenger.Envelope, next messenger.MiddlewareNext) (messenger.DispatchResult, error) {
	m.called.Store(true)
	return next(ctx, env)
}

func TestConditional_Command_RunsForCommand(t *testing.T) {
	reg := messenger.NewHandlerRegistry()
	messenger.RegisterCommandFunc[testCmd](reg, func(_ context.Context, _ testCmd) error { return nil })

	inner := &trackingMiddleware{}
	cond := middleware.WhenCommand(inner, reg)

	env := messenger.NewEnvelope(testCmd{ID: "1"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called.Load() {
		t.Error("inner middleware should be called for command")
	}
}

func TestConditional_Command_SkipsForQuery(t *testing.T) {
	reg := messenger.NewHandlerRegistry()
	messenger.RegisterCommandFunc[testCmd](reg, func(_ context.Context, _ testCmd) error { return nil })
	messenger.RegisterQueryFunc[testQuery, string](reg, func(_ context.Context, _ testQuery) (string, error) { return "ok", nil })

	inner := &trackingMiddleware{}
	cond := middleware.WhenCommand(inner, reg)

	env := messenger.NewEnvelope(testQuery{ID: "1"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called.Load() {
		t.Error("inner middleware should NOT be called for query")
	}
}

func TestConditional_Query_RunsForQuery(t *testing.T) {
	reg := messenger.NewHandlerRegistry()
	messenger.RegisterQueryFunc[testQuery, string](reg, func(_ context.Context, _ testQuery) (string, error) { return "ok", nil })

	inner := &trackingMiddleware{}
	cond := middleware.WhenQuery(inner, reg)

	env := messenger.NewEnvelope(testQuery{ID: "1"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called.Load() {
		t.Error("inner middleware should be called for query")
	}
}

func TestConditional_Query_SkipsForCommand(t *testing.T) {
	reg := messenger.NewHandlerRegistry()
	messenger.RegisterCommandFunc[testCmd](reg, func(_ context.Context, _ testCmd) error { return nil })

	inner := &trackingMiddleware{}
	cond := middleware.WhenQuery(inner, reg)

	env := messenger.NewEnvelope(testCmd{ID: "1"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called.Load() {
		t.Error("inner middleware should NOT be called for command")
	}
}

func TestConditional_Consumed_RunsWithReceivedStamp(t *testing.T) {
	inner := &trackingMiddleware{}
	cond := middleware.WhenConsumed(inner)

	env := messenger.NewEnvelope(testCmd{ID: "1"}, stamp.ReceivedStamp{Transport: "memory"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called.Load() {
		t.Error("inner middleware should be called when ReceivedStamp is present")
	}
}

func TestConditional_Consumed_SkipsWithoutReceivedStamp(t *testing.T) {
	inner := &trackingMiddleware{}
	cond := middleware.WhenConsumed(inner)

	env := messenger.NewEnvelope(testCmd{ID: "1"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called.Load() {
		t.Error("inner middleware should NOT be called without ReceivedStamp")
	}
}

func TestConditional_Always_AlwaysRuns(t *testing.T) {
	inner := &trackingMiddleware{}
	cond := middleware.NewConditional(inner, middleware.Always, nil)

	env := messenger.NewEnvelope(testCmd{ID: "1"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called.Load() {
		t.Error("inner middleware should always be called with Always condition")
	}
}

func TestConditional_NilRegistry_SafeFallback(t *testing.T) {
	inner := &trackingMiddleware{}
	cond := middleware.NewConditional(inner, middleware.OnCommand, nil)

	env := messenger.NewEnvelope(testCmd{ID: "1"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called.Load() {
		t.Error("should skip when registry is nil")
	}
}

func TestConditional_Sync_RunsWithoutAsyncStamps(t *testing.T) {
	inner := &trackingMiddleware{}
	cond := middleware.NewConditional(inner, middleware.OnSync, nil)

	env := messenger.NewEnvelope(testCmd{ID: "1"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called.Load() {
		t.Error("Sync condition should run without async stamps")
	}
}

func TestConditional_Sync_SkipsWithSentStamp(t *testing.T) {
	inner := &trackingMiddleware{}
	cond := middleware.NewConditional(inner, middleware.OnSync, nil)

	env := messenger.NewEnvelope(testCmd{ID: "1"}, stamp.SentStamp{Transport: "nats"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inner.called.Load() {
		t.Error("Sync condition should skip when SentStamp present")
	}
}

func TestConditional_Async_RunsWithSentStamp(t *testing.T) {
	inner := &trackingMiddleware{}
	cond := middleware.NewConditional(inner, middleware.OnAsync, nil)

	env := messenger.NewEnvelope(testCmd{ID: "1"}, stamp.SentStamp{Transport: "nats"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called.Load() {
		t.Error("Async condition should run with SentStamp")
	}
}

func TestConditional_Async_RunsWithReceivedStamp(t *testing.T) {
	inner := &trackingMiddleware{}
	cond := middleware.NewConditional(inner, middleware.OnAsync, nil)

	env := messenger.NewEnvelope(testCmd{ID: "1"}, stamp.ReceivedStamp{Transport: "nats"})
	_, err := cond.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inner.called.Load() {
		t.Error("Async condition should run with ReceivedStamp")
	}
}

func BenchmarkConditional_CommandCheck(b *testing.B) {
	reg := messenger.NewHandlerRegistry()
	messenger.RegisterCommandFunc[testCmd](reg, func(_ context.Context, _ testCmd) error { return nil })

	inner := &trackingMiddleware{}
	cond := middleware.WhenCommand(inner, reg)
	env := messenger.NewEnvelope(testCmd{ID: "bench"})
	ctx := context.Background()

	b.ResetTimer()
	for range b.N {
		_, _ = cond.Handle(ctx, env, passThrough)
	}
}
