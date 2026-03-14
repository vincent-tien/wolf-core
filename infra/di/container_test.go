package di_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/di"
)

// --- Singleton tests --------------------------------------------------------

func TestSingleton_ReturnsSameInstance(t *testing.T) {
	ctr := di.New()

	var calls int

	ctr.AddSingleton("svc", func(_ di.Container) any {
		calls++
		return &struct{ ID int }{ID: 42}
	})

	first := ctr.Get("svc")
	second := ctr.Get("svc")

	assert.Same(t, first, second, "singleton must return the same pointer")
	assert.Equal(t, 1, calls, "factory must be called exactly once")
}

func TestSingleton_SharedAcrossScopes(t *testing.T) {
	ctr := di.New()

	ctr.AddSingleton("shared", func(_ di.Container) any {
		return &struct{}{}
	})

	ctx1 := ctr.Scoped(context.Background())
	ctx2 := ctr.Scoped(context.Background())

	v1 := di.FromContext(ctx1).Get("shared")
	v2 := di.FromContext(ctx2).Get("shared")

	assert.Same(t, v1, v2, "singleton must be the same instance across different scopes")
}

// --- Scoped tests -----------------------------------------------------------

func TestScoped_DifferentInstancesPerScope(t *testing.T) {
	ctr := di.New()

	// Use a struct with a non-zero-size field so the Go runtime allocates
	// a distinct pointer for each instance (zero-size structs may share addresses).
	type instance struct{ id int }
	var n int

	ctr.AddScoped("req", func(_ di.Container) any {
		n++
		return &instance{id: n}
	})

	ctx1 := ctr.Scoped(context.Background())
	ctx2 := ctr.Scoped(context.Background())

	v1 := di.FromContext(ctx1).Get("req").(*instance)
	v2 := di.FromContext(ctx2).Get("req").(*instance)

	assert.NotSame(t, v1, v2, "scoped factory must produce a distinct instance per scope")
	assert.NotEqual(t, v1.id, v2.id, "each scope must receive a fresh instance from the factory")
}

func TestScoped_SameInstanceWithinScope(t *testing.T) {
	ctr := di.New()

	var calls int

	type instance struct{ n int }

	ctr.AddScoped("req", func(_ di.Container) any {
		calls++
		return &instance{n: calls}
	})

	ctx := ctr.Scoped(context.Background())
	c := di.FromContext(ctx)

	first := c.Get("req").(*instance)
	second := c.Get("req").(*instance)

	assert.Same(t, first, second, "scoped factory must return the same instance within one scope")
	assert.Equal(t, 1, calls, "factory must be called once within a scope")
}

// --- Unregistered key -------------------------------------------------------

func TestGet_UnregisteredKey_Panics(t *testing.T) {
	ctr := di.New()

	assert.Panics(t, func() {
		ctr.Get("missing")
	}, "Get with an unregistered key must panic")
}

func TestGetTyped_UnregisteredKey_Panics(t *testing.T) {
	ctr := di.New()
	ctx := ctr.Scoped(context.Background())

	assert.Panics(t, func() {
		di.GetTyped[string](ctx, "missing")
	})
}

// --- GetTyped ---------------------------------------------------------------

func TestGetTyped_ReturnsCorrectType(t *testing.T) {
	ctr := di.New()

	ctr.AddSingleton("greeting", func(_ di.Container) any {
		return "hello"
	})

	ctx := ctr.Scoped(context.Background())

	got := di.GetTyped[string](ctx, "greeting")

	assert.Equal(t, "hello", got)
}

func TestGetTyped_WrongType_Panics(t *testing.T) {
	ctr := di.New()

	ctr.AddSingleton("num", func(_ di.Container) any {
		return 42
	})

	ctx := ctr.Scoped(context.Background())

	assert.Panics(t, func() {
		di.GetTyped[string](ctx, "num")
	})
}

func TestGetTyped_NoScopedContainer_Panics(t *testing.T) {
	ctr := di.New()
	ctr.AddSingleton("x", func(_ di.Container) any { return "v" })

	// Context has no scope — FromContext returns nil.
	assert.Panics(t, func() {
		di.GetTyped[string](context.Background(), "x")
	})
}

// --- FromContext ------------------------------------------------------------

func TestFromContext_NoContainer_ReturnsNil(t *testing.T) {
	c := di.FromContext(context.Background())

	assert.Nil(t, c)
}

func TestFromContext_WithScope_ReturnsContainer(t *testing.T) {
	ctr := di.New()
	ctx := ctr.Scoped(context.Background())

	c := di.FromContext(ctx)

	require.NotNil(t, c)
}

// --- Dependency resolution via factory arg ----------------------------------

func TestFactory_ReceivesContainerForGraphAssembly(t *testing.T) {
	ctr := di.New()

	ctr.AddSingleton("base", func(_ di.Container) any {
		return "base-value"
	})

	ctr.AddSingleton("derived", func(c di.Container) any {
		base := c.Get("base").(string)
		return "derived-from-" + base
	})

	got := ctr.Get("derived")

	assert.Equal(t, "derived-from-base-value", got)
}

// --- Concurrent access -------------------------------------------------------

func TestSingleton_ConcurrentAccess_NoDuplication(t *testing.T) {
	ctr := di.New()

	var calls atomic.Int64

	ctr.AddSingleton("heavy", func(_ di.Container) any {
		calls.Add(1)
		return &struct{}{}
	})

	const goroutines = 50

	var wg sync.WaitGroup

	results := make([]any, goroutines)

	for i := range goroutines {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()
			results[idx] = ctr.Get("heavy")
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int64(1), calls.Load(), "singleton factory must be called exactly once under concurrent access")

	for i := 1; i < goroutines; i++ {
		assert.Same(t, results[0], results[i], "all goroutines must receive the same singleton instance")
	}
}

// --- GetTypedSafe -----------------------------------------------------------

func TestGetTypedSafe_ReturnsValue(t *testing.T) {
	ctr := di.New()
	ctr.AddSingleton("greeting", func(_ di.Container) any { return "hello" })
	ctx := ctr.Scoped(context.Background())

	got, err := di.GetTypedSafe[string](ctx, "greeting")

	require.NoError(t, err)
	assert.Equal(t, "hello", got)
}

func TestGetTypedSafe_NoContainer_ReturnsError(t *testing.T) {
	_, err := di.GetTypedSafe[string](context.Background(), "x")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no scoped container")
}

func TestGetTypedSafe_TypeMismatch_ReturnsError(t *testing.T) {
	ctr := di.New()
	ctr.AddSingleton("num", func(_ di.Container) any { return 42 })
	ctx := ctr.Scoped(context.Background())

	_, err := di.GetTypedSafe[string](ctx, "num")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "type assertion failed")
}

// --- Validate ----------------------------------------------------------------

func TestValidate_AllSingletonsResolve_ReturnsNil(t *testing.T) {
	ctr := di.New()
	ctr.AddSingleton("a", func(_ di.Container) any { return "hello" })
	ctr.AddSingleton("b", func(c di.Container) any { return c.Get("a").(string) + " world" })

	err := ctr.Validate()

	assert.NoError(t, err)
}

func TestValidate_MissingDependency_ReturnsError(t *testing.T) {
	ctr := di.New()
	ctr.AddSingleton("broken", func(c di.Container) any {
		return c.Get("does_not_exist") // panics
	})

	err := ctr.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken")
	assert.Contains(t, err.Error(), "panicked")
}

func TestValidate_SkipsScoped(t *testing.T) {
	ctr := di.New()
	ctr.AddScoped("scoped", func(c di.Container) any {
		return c.Get("missing") // would panic if resolved
	})

	err := ctr.Validate()

	assert.NoError(t, err, "scoped entries must be skipped during validation")
}

func TestValidate_CollectsMultipleErrors(t *testing.T) {
	ctr := di.New()
	ctr.AddSingleton("bad1", func(c di.Container) any { return c.Get("x") })
	ctr.AddSingleton("bad2", func(c di.Container) any { return c.Get("y") })

	err := ctr.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad1")
	assert.Contains(t, err.Error(), "bad2")
}

func TestScoped_ConcurrentAccessWithinScope_NoDuplication(t *testing.T) {
	ctr := di.New()

	var calls atomic.Int64

	ctr.AddScoped("conn", func(_ di.Container) any {
		calls.Add(1)
		return &struct{}{}
	})

	ctx := ctr.Scoped(context.Background())
	c := di.FromContext(ctx)

	const goroutines = 50

	var wg sync.WaitGroup

	results := make([]any, goroutines)

	for i := range goroutines {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()
			results[idx] = c.Get("conn")
		}(i)
	}

	wg.Wait()

	assert.Equal(t, int64(1), calls.Load(), "scoped factory must be called exactly once within a single scope under concurrent access")

	for i := 1; i < goroutines; i++ {
		assert.Same(t, results[0], results[i])
	}
}
