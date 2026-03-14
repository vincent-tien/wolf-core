package messenger

import (
	"context"
	"testing"

	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

func terminalOK(_ context.Context, env Envelope) (DispatchResult, error) {
	return DispatchResult{Envelope: env}, nil
}

func TestChain_EmptyChain_CallsTerminal(t *testing.T) {
	var called bool
	terminal := func(_ context.Context, env Envelope) (DispatchResult, error) {
		called = true
		return DispatchResult{Envelope: env}, nil
	}

	chain := buildChain(nil, terminal)
	env := NewEnvelope(testMsg{ID: "1"})
	_, err := chain.execute(context.Background(), env)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !called {
		t.Error("terminal was not called")
	}
}

func TestChain_SingleMiddleware(t *testing.T) {
	var order []string

	mw := MiddlewareFunc(func(ctx context.Context, env Envelope, next MiddlewareNext) (DispatchResult, error) {
		order = append(order, "mw-before")
		result, err := next(ctx, env)
		order = append(order, "mw-after")
		return result, err
	})

	terminal := func(_ context.Context, env Envelope) (DispatchResult, error) {
		order = append(order, "terminal")
		return DispatchResult{Envelope: env}, nil
	}

	chain := buildChain([]Middleware{mw}, terminal)
	_, err := chain.execute(context.Background(), NewEnvelope(testMsg{ID: "1"}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	expected := []string{"mw-before", "terminal", "mw-after"}
	if len(order) != len(expected) {
		t.Fatalf("order = %v, want %v", order, expected)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}

func TestChain_FiveMiddleware_CorrectOrder(t *testing.T) {
	var order []int

	makeMW := func(id int) Middleware {
		return MiddlewareFunc(func(ctx context.Context, env Envelope, next MiddlewareNext) (DispatchResult, error) {
			order = append(order, id)
			return next(ctx, env)
		})
	}

	mws := []Middleware{makeMW(1), makeMW(2), makeMW(3), makeMW(4), makeMW(5)}
	terminal := func(_ context.Context, env Envelope) (DispatchResult, error) {
		order = append(order, 0)
		return DispatchResult{Envelope: env}, nil
	}

	chain := buildChain(mws, terminal)
	_, err := chain.execute(context.Background(), NewEnvelope(testMsg{ID: "1"}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	expected := []int{1, 2, 3, 4, 5, 0}
	if len(order) != len(expected) {
		t.Fatalf("order = %v, want %v", order, expected)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %d, want %d", i, order[i], v)
		}
	}
}

func TestChain_ShortCircuit(t *testing.T) {
	var terminalCalled bool

	mw := MiddlewareFunc(func(_ context.Context, env Envelope, _ MiddlewareNext) (DispatchResult, error) {
		return DispatchResult{Envelope: env}, nil // don't call next
	})

	terminal := func(_ context.Context, env Envelope) (DispatchResult, error) {
		terminalCalled = true
		return DispatchResult{Envelope: env}, nil
	}

	chain := buildChain([]Middleware{mw}, terminal)
	_, err := chain.execute(context.Background(), NewEnvelope(testMsg{ID: "1"}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if terminalCalled {
		t.Error("terminal should not be called when middleware short-circuits")
	}
}

func TestChain_MiddlewareModifiesEnvelope(t *testing.T) {
	mw := MiddlewareFunc(func(ctx context.Context, env Envelope, next MiddlewareNext) (DispatchResult, error) {
		env = env.WithStamp(stamp.BusNameStamp{Name: "added-by-mw"})
		return next(ctx, env)
	})

	terminal := func(_ context.Context, env Envelope) (DispatchResult, error) {
		if !env.HasStamp(stamp.NameBusName) {
			return DispatchResult{}, errMissingStamp
		}
		return DispatchResult{Envelope: env}, nil
	}

	chain := buildChain([]Middleware{mw}, terminal)
	result, err := chain.execute(context.Background(), NewEnvelope(testMsg{ID: "1"}))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !result.Envelope.HasStamp(stamp.NameBusName) {
		t.Error("stamp added by middleware not found in result")
	}
}

var errMissingStamp = errSentinel("missing stamp")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

// noopMiddleware for benchmarks — passes through with zero work.
type noopMiddleware struct{}

func (noopMiddleware) Handle(ctx context.Context, env Envelope, next MiddlewareNext) (DispatchResult, error) {
	return next(ctx, env)
}

// ── Benchmarks ──

func BenchmarkChainExecute_0MW(b *testing.B) {
	chain := buildChain(nil, terminalOK)
	env := NewEnvelope(testMsg{ID: "bench"})
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		chain.execute(ctx, env)
	}
}

func BenchmarkChainExecute_1MW(b *testing.B) {
	chain := buildChain([]Middleware{noopMiddleware{}}, terminalOK)
	env := NewEnvelope(testMsg{ID: "bench"})
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		chain.execute(ctx, env)
	}
}

func BenchmarkChainExecute_5MW(b *testing.B) {
	mws := []Middleware{
		noopMiddleware{}, noopMiddleware{}, noopMiddleware{},
		noopMiddleware{}, noopMiddleware{},
	}
	chain := buildChain(mws, terminalOK)
	env := NewEnvelope(testMsg{ID: "bench"})
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		chain.execute(ctx, env)
	}
}

func BenchmarkChainExecute_5MW_Parallel(b *testing.B) {
	mws := []Middleware{
		noopMiddleware{}, noopMiddleware{}, noopMiddleware{},
		noopMiddleware{}, noopMiddleware{},
	}
	chain := buildChain(mws, terminalOK)
	env := NewEnvelope(testMsg{ID: "bench"})
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			chain.execute(ctx, env)
		}
	})
}
