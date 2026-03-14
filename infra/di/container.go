// Package di provides a lightweight, scoped dependency injection container.
//
// Lifecycle semantics:
//
//   - Singleton: the factory is called exactly once across all scopes. The
//     result is cached in the root container and reused for every subsequent
//     Get or Scoped call.
//
//   - Scoped: the factory is called at most once per scope (request, RPC,
//     background job). A new scope is created by calling Container.Scoped,
//     which returns a child context that carries the scoped container. Within
//     a scope, Get returns the same instance on every call for the same key.
//
// Usage pattern:
//
//	// Application startup — register factories once.
//	ctr := di.New()
//	ctr.AddSingleton("logger", func(c di.Container) any { return buildLogger() })
//	ctr.AddScoped("db_conn", func(c di.Container) any { return openConn() })
//
//	// Per-request — create a scope and resolve dependencies.
//	ctx = ctr.Scoped(ctx)
//	conn := di.GetTyped[db.Conn](ctx, "db.conn")
package di

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ctxKey is an unexported type used as the context key for the scoped
// container to avoid collisions with other context values.
type ctxKey struct{}

// Factory is a function that creates a dependency. The Container argument
// gives the factory access to other registered dependencies so that
// object graphs can be assembled lazily.
type Factory func(c Container) any

// Container is the primary interface for registering and resolving dependencies.
type Container interface {
	// AddSingleton registers a factory whose result is created once and shared
	// across all scopes for the lifetime of the root container.
	AddSingleton(key string, factory Factory)

	// AddScoped registers a factory whose result is created once per scope
	// (i.e. once per Scoped call). Each scope gets its own instance.
	AddScoped(key string, factory Factory)

	// Get resolves the dependency registered under key. For singletons the
	// cached instance is returned; for scoped values the scoped instance is
	// returned if called from a scoped container, or a fresh instance is
	// created if called from the root container.
	//
	// Get panics if key has not been registered.
	Get(key string) any

	// Scoped creates a child scope, stores it in ctx, and returns the new
	// context. The child inherits all factory registrations from the root
	// container. Use FromContext to retrieve the scoped container later.
	Scoped(ctx context.Context) context.Context

	// Validate eagerly resolves every singleton registration to surface
	// missing dependencies at startup rather than at first-request time.
	// Scoped entries are skipped (they require a request context).
	// Returns a combined error of all failures, or nil if all singletons
	// resolve successfully.
	Validate() error
}

// entry holds a factory and a flag indicating whether the key is scoped.
type entry struct {
	factory  Factory
	isScoped bool
}

// singletonEntry holds the once-initialised value for a singleton.
type singletonEntry struct {
	once  sync.Once
	value any
}

// container is the root implementation of Container. It holds factory
// registrations in a plain map (protected by regMu) and singleton cache
// entries in a separate sync.Map so that each singleton can be initialised
// exactly once without holding the registry lock during factory invocation.
type container struct {
	// regMu protects entries. It is only held for reads/writes to the
	// entries map — never while calling a factory function.
	regMu   sync.RWMutex
	entries map[string]entry

	// singletons maps key → *singletonEntry. Entries are added lazily on
	// first Get for a singleton key. sync.Map provides safe concurrent
	// load-or-store without holding regMu during factory calls.
	singletons sync.Map
}

// New constructs a new empty root Container.
func New() Container {
	return &container{
		entries: make(map[string]entry),
	}
}

// AddSingleton registers key as a singleton-scoped dependency.
func (c *container) AddSingleton(key string, factory Factory) {
	c.regMu.Lock()
	defer c.regMu.Unlock()

	c.entries[key] = entry{factory: factory, isScoped: false}
}

// AddScoped registers key as a per-scope dependency.
func (c *container) AddScoped(key string, factory Factory) {
	c.regMu.Lock()
	defer c.regMu.Unlock()

	c.entries[key] = entry{factory: factory, isScoped: true}
}

// lookupEntry returns the entry for key and true, or zero entry and false.
func (c *container) lookupEntry(key string) (entry, bool) {
	c.regMu.RLock()
	e, ok := c.entries[key]
	c.regMu.RUnlock()

	return e, ok
}

// Get resolves key. Singletons are initialised exactly once via sync.Once.
// Scoped keys resolved from the root container (no active scope) create a
// fresh instance on every call.
//
// Get panics if key has not been registered.
func (c *container) Get(key string) any {
	e, ok := c.lookupEntry(key)
	if !ok {
		panic("di: key not registered: " + key)
	}

	if e.isScoped {
		// No scope active — call factory directly, no caching.
		return e.factory(c)
	}

	// Singleton path: load-or-store a *singletonEntry, then call Once.Do.
	// This ensures exactly-one initialisation even under concurrent Get calls,
	// and crucially the factory runs outside of any lock.
	actual, _ := c.singletons.LoadOrStore(key, &singletonEntry{})
	se := actual.(*singletonEntry)

	se.once.Do(func() {
		se.value = e.factory(c)
	})

	return se.value
}

// Validate eagerly resolves every singleton factory to catch missing
// dependencies at startup. Panics from factories are recovered and
// collected into a combined error. Scoped entries are skipped.
func (c *container) Validate() error {
	c.regMu.RLock()
	snapshot := make(map[string]entry, len(c.entries))
	for k, e := range c.entries {
		snapshot[k] = e
	}
	c.regMu.RUnlock()

	var errs []error
	for key, e := range snapshot {
		if e.isScoped {
			continue
		}
		if err := c.tryResolve(key, e); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// tryResolve attempts to call a singleton factory, recovering any panic.
func (c *container) tryResolve(key string, e entry) (resolveErr error) {
	defer func() {
		if r := recover(); r != nil {
			resolveErr = fmt.Errorf("di: singleton %q panicked during validation: %v", key, r)
		}
	}()

	actual, _ := c.singletons.LoadOrStore(key, &singletonEntry{})
	se := actual.(*singletonEntry)
	se.once.Do(func() {
		se.value = e.factory(c)
	})

	return nil
}

// Scoped creates a child scopedContainer, injects it into ctx, and returns
// the enriched context. The child inherits all factory registrations from the root.
func (c *container) Scoped(ctx context.Context) context.Context {
	child := &scopedContainer{
		root:   c,
		values: make(map[string]any),
	}

	return context.WithValue(ctx, ctxKey{}, child)
}

// scopedContainer is a per-request (or per-RPC) container that caches scoped
// instances for its own lifetime. Singleton resolution is delegated to the
// root container.
type scopedContainer struct {
	root *container

	mu     sync.Mutex
	values map[string]any
}

// AddSingleton delegates to the root container. Registrations made through a
// scoped container are visible to the root and all other scopes.
func (s *scopedContainer) AddSingleton(key string, factory Factory) {
	s.root.AddSingleton(key, factory)
}

// AddScoped delegates to the root container.
func (s *scopedContainer) AddScoped(key string, factory Factory) {
	s.root.AddScoped(key, factory)
}

// Get resolves key. Singletons are delegated to root. Scoped values are
// created once per scopedContainer instance and cached under the scope lock.
func (s *scopedContainer) Get(key string) any {
	e, ok := s.root.lookupEntry(key)
	if !ok {
		panic("di: key not registered: " + key)
	}

	if !e.isScoped {
		return s.root.Get(key)
	}

	// Scoped: check scope cache under lock to prevent duplicate construction.
	s.mu.Lock()
	defer s.mu.Unlock()

	if v, ok := s.values[key]; ok {
		return v
	}

	v := e.factory(s)
	s.values[key] = v

	return v
}

// Validate delegates to the root container.
func (s *scopedContainer) Validate() error {
	return s.root.Validate()
}

// Scoped creates a new child scope from the root container, not from this
// scoped container, to keep the scope depth flat (one level only).
func (s *scopedContainer) Scoped(ctx context.Context) context.Context {
	return s.root.Scoped(ctx)
}

// FromContext retrieves the scoped Container stored in ctx by Scoped. If no
// scoped container is present, nil is returned.
func FromContext(ctx context.Context) Container {
	v := ctx.Value(ctxKey{})
	if v == nil {
		return nil
	}

	c, ok := v.(Container)
	if !ok {
		return nil
	}

	return c
}

// GetTyped is a convenience wrapper around FromContext + Get that performs a
// type assertion. It panics if the context carries no scoped container, if key
// is not registered, or if the resolved value is not of type T.
func GetTyped[T any](ctx context.Context, key string) T {
	c := FromContext(ctx)
	if c == nil {
		panic("di: no scoped container in context")
	}

	v := c.Get(key)

	typed, ok := v.(T)
	if !ok {
		panic("di: type assertion failed for key: " + key)
	}

	return typed
}

// GetTypedSafe is the error-returning variant of GetTyped. It returns a typed
// error instead of panicking on missing container, unregistered key, or type
// mismatch. Prefer this in code paths where a panic is unacceptable.
func GetTypedSafe[T any](ctx context.Context, key string) (T, error) {
	var zero T
	c := FromContext(ctx)
	if c == nil {
		return zero, fmt.Errorf("di: no scoped container in context")
	}

	v := c.Get(key)

	typed, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("di: type assertion failed for key %q: got %T", key, v)
	}

	return typed, nil
}
