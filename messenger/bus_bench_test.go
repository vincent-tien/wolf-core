package messenger

import (
	"context"
	"testing"
)

// benchCmd is a minimal command for benchmarking.
type benchCmd struct{}

func (benchCmd) MessageName() string { return "bench.Cmd" }

type benchQuery struct{}

func (benchQuery) MessageName() string { return "bench.Query" }

func newSyncBus(mws ...Middleware) *MessageBus {
	bus := NewBus("bench", WithMiddleware(mws...))
	RegisterCommandFunc[benchCmd](bus.Handlers(), func(_ context.Context, _ benchCmd) error {
		return nil
	})
	RegisterQueryFunc[benchQuery, int](bus.Handlers(), func(_ context.Context, _ benchQuery) (int, error) {
		return 42, nil
	})
	return bus
}

func BenchmarkSyncDispatch_NoStamps_NoMW(b *testing.B) {
	bus := newSyncBus()
	ctx := context.Background()
	cmd := benchCmd{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		bus.Dispatch(ctx, cmd)
	}
}

func BenchmarkSyncDispatch_NoStamps_5MW(b *testing.B) {
	bus := newSyncBus(
		noopMiddleware{}, noopMiddleware{}, noopMiddleware{},
		noopMiddleware{}, noopMiddleware{},
	)
	ctx := context.Background()
	cmd := benchCmd{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		bus.Dispatch(ctx, cmd)
	}
}

func BenchmarkSyncDispatch_2Stamps_5MW(b *testing.B) {
	bus := newSyncBus(
		noopMiddleware{}, noopMiddleware{}, noopMiddleware{},
		noopMiddleware{}, noopMiddleware{},
	)
	ctx := context.Background()
	cmd := benchCmd{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		bus.Dispatch(ctx, cmd)
	}
}

func BenchmarkQueryDispatch(b *testing.B) {
	bus := newSyncBus()
	ctx := context.Background()
	query := benchQuery{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		bus.Query(ctx, query)
	}
}

func BenchmarkSyncDispatch_Parallel(b *testing.B) {
	bus := newSyncBus(
		noopMiddleware{}, noopMiddleware{}, noopMiddleware{},
	)
	ctx := context.Background()
	cmd := benchCmd{}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bus.Dispatch(ctx, cmd)
		}
	})
}
