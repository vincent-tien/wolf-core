package messenger

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// ── Test message types ──

type createUserCmd struct{ Name string }

func (createUserCmd) MessageName() string { return "iam.CreateUserCmd" }

type deleteUserCmd struct{ ID string }

func (deleteUserCmd) MessageName() string { return "iam.DeleteUserCmd" }

type getUserQuery struct{ ID string }

func (getUserQuery) MessageName() string { return "iam.GetUserQuery" }

type userResponse struct{ Name string }

type unknownMsg struct{}

func (unknownMsg) MessageName() string { return "unknown.Msg" }

// ── Tests ──

func TestRegisterCommand_Resolve(t *testing.T) {
	reg := NewHandlerRegistry()
	var called bool

	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, cmd createUserCmd) error {
		called = true
		if cmd.Name != "alice" {
			t.Errorf("cmd.Name = %q, want %q", cmd.Name, "alice")
		}
		return nil
	})

	h, err := reg.Resolve(createUserCmd{Name: "alice"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !h.IsCommand() {
		t.Error("expected IsCommand() = true")
	}

	_, err = h.HandleAny(context.Background(), createUserCmd{Name: "alice"})
	if err != nil {
		t.Fatalf("HandleAny: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
}

func TestRegisterQuery_Resolve(t *testing.T) {
	reg := NewHandlerRegistry()

	RegisterQueryFunc[getUserQuery, userResponse](reg, func(_ context.Context, q getUserQuery) (userResponse, error) {
		return userResponse{Name: "alice"}, nil
	})

	h, err := reg.Resolve(getUserQuery{ID: "1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if h.IsCommand() {
		t.Error("expected IsCommand() = false for query")
	}

	result, err := h.HandleAny(context.Background(), getUserQuery{ID: "1"})
	if err != nil {
		t.Fatalf("HandleAny: %v", err)
	}
	resp, ok := result.(userResponse)
	if !ok {
		t.Fatalf("result type = %T, want userResponse", result)
	}
	if resp.Name != "alice" {
		t.Errorf("resp.Name = %q, want %q", resp.Name, "alice")
	}
}

func TestDuplicateRegistration_Panics(t *testing.T) {
	reg := NewHandlerRegistry()
	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, _ createUserCmd) error {
		return nil
	})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()

	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, _ createUserCmd) error {
		return nil
	})
}

func TestResolve_Unregistered_ReturnsErrNoHandler(t *testing.T) {
	reg := NewHandlerRegistry()
	_, err := reg.Resolve(unknownMsg{})
	if !errors.Is(err, ErrNoHandler) {
		t.Errorf("err = %v, want ErrNoHandler", err)
	}
}

func TestHas(t *testing.T) {
	reg := NewHandlerRegistry()
	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, _ createUserCmd) error {
		return nil
	})

	if !reg.Has(createUserCmd{}) {
		t.Error("Has(createUserCmd) = false, want true")
	}
	if reg.Has(unknownMsg{}) {
		t.Error("Has(unknownMsg) = true, want false")
	}
}

func TestRegisteredTypes(t *testing.T) {
	reg := NewHandlerRegistry()
	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, _ createUserCmd) error {
		return nil
	})
	RegisterQueryFunc[getUserQuery, userResponse](reg, func(_ context.Context, _ getUserQuery) (userResponse, error) {
		return userResponse{}, nil
	})

	types := reg.RegisteredTypes()
	if len(types) != 2 {
		t.Errorf("RegisteredTypes() len = %d, want 2", len(types))
	}
}

func TestCommandTypes_QueryTypes_Separation(t *testing.T) {
	reg := NewHandlerRegistry()
	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, _ createUserCmd) error { return nil })
	RegisterCommandFunc[deleteUserCmd](reg, func(_ context.Context, _ deleteUserCmd) error { return nil })
	RegisterQueryFunc[getUserQuery, userResponse](reg, func(_ context.Context, _ getUserQuery) (userResponse, error) {
		return userResponse{}, nil
	})

	cmdTypes := reg.CommandTypes()
	if len(cmdTypes) != 2 {
		t.Errorf("CommandTypes() len = %d, want 2", len(cmdTypes))
	}

	qTypes := reg.QueryTypes()
	if len(qTypes) != 1 {
		t.Errorf("QueryTypes() len = %d, want 1", len(qTypes))
	}
}

func TestConcurrentReads_Safe(t *testing.T) {
	reg := NewHandlerRegistry()
	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, _ createUserCmd) error { return nil })

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				_, _ = reg.Resolve(createUserCmd{})
				_ = reg.Has(createUserCmd{})
			}
		}()
	}
	wg.Wait()
}

func TestHandleAny_WrongType_ReturnsError(t *testing.T) {
	reg := NewHandlerRegistry()
	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, _ createUserCmd) error { return nil })

	h, err := reg.Resolve(createUserCmd{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Pass wrong type to HandleAny.
	_, err = h.HandleAny(context.Background(), "not a createUserCmd")
	if err == nil {
		t.Error("expected error for wrong type assertion")
	}
}

// ── Benchmarks ──

func BenchmarkRegistryResolve(b *testing.B) {
	reg := NewHandlerRegistry()
	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, _ createUserCmd) error { return nil })
	msg := createUserCmd{Name: "bench"}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		reg.Resolve(msg)
	}
}

func BenchmarkRegistryResolve_Miss(b *testing.B) {
	reg := NewHandlerRegistry()
	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, _ createUserCmd) error { return nil })
	msg := unknownMsg{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		reg.Resolve(msg)
	}
}

func BenchmarkRegistryResolve_Parallel(b *testing.B) {
	reg := NewHandlerRegistry()
	RegisterCommandFunc[createUserCmd](reg, func(_ context.Context, _ createUserCmd) error { return nil })
	msg := createUserCmd{Name: "bench"}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			reg.Resolve(msg)
		}
	})
}
