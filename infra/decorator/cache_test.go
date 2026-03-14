package decorator_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/cache"
	"github.com/vincent-tien/wolf-core/infra/decorator"
)

// ---------------------------------------------------------------------------
// In-memory cache stub
// ---------------------------------------------------------------------------

type stubCache struct {
	mu       sync.Mutex
	store    map[string][]byte
	getErr   error
	setErr   error
	setCalls int
}

func newStubCache() *stubCache {
	return &stubCache{store: make(map[string][]byte)}
}

func (s *stubCache) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.getErr != nil {
		return nil, s.getErr
	}
	v, ok := s.store[key]
	if !ok {
		return nil, cache.ErrCacheMiss
	}
	return v, nil
}

func (s *stubCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.store[key] = value
	return nil
}

func (s *stubCache) Delete(_ context.Context, _ ...string) error { return nil }
func (s *stubCache) Ping(_ context.Context) error                { return nil }
func (s *stubCache) Close() error                                { return nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type product struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func keyFn(_ string) string { return "product:1" }

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestWithCache_Hit(t *testing.T) {
	t.Parallel()

	// Arrange
	c := newStubCache()
	expected := product{ID: 1, Name: "Widget"}
	c.store["product:1"] = mustMarshal(t, expected)

	fnCalled := false
	fn := decorator.Func[string, product](func(_ context.Context, _ string) (product, error) {
		fnCalled = true
		return product{}, errors.New("should not be called")
	})

	mw := decorator.WithCache[string, product](c, keyFn, time.Minute, zap.NewNop())
	wrapped := decorator.Chain(fn, mw)

	// Act
	got, err := wrapped(context.Background(), "1")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expected, got)
	assert.False(t, fnCalled, "fn must not be called on cache hit")
}

func TestWithCache_MissAndPopulate(t *testing.T) {
	t.Parallel()

	// Arrange
	c := newStubCache()
	expected := product{ID: 2, Name: "Gadget"}

	fn := decorator.Func[string, product](func(_ context.Context, _ string) (product, error) {
		return expected, nil
	})

	mw := decorator.WithCache[string, product](c, keyFn, time.Minute, zap.NewNop())
	wrapped := decorator.Chain(fn, mw)

	// Act
	got, err := wrapped(context.Background(), "2")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expected, got)
	assert.Equal(t, 1, c.setCalls, "result must be written to cache on miss")

	var stored product
	require.NoError(t, json.Unmarshal(c.store["product:1"], &stored))
	assert.Equal(t, expected, stored)
}

func TestWithCache_ErrorNonFatal(t *testing.T) {
	t.Parallel()

	// Arrange — cache.Get returns a non-miss error; fn must still be called.
	c := newStubCache()
	c.getErr = errors.New("redis unavailable")
	expected := product{ID: 3, Name: "Thing"}

	fnCalled := false
	fn := decorator.Func[string, product](func(_ context.Context, _ string) (product, error) {
		fnCalled = true
		return expected, nil
	})

	mw := decorator.WithCache[string, product](c, keyFn, time.Minute, zap.NewNop())
	wrapped := decorator.Chain(fn, mw)

	// Act
	got, err := wrapped(context.Background(), "3")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expected, got)
	assert.True(t, fnCalled, "fn must be called when cache.Get errors")
}

func TestWithCache_FnErrorNotCached(t *testing.T) {
	t.Parallel()

	// Arrange — fn returns an error; cache.Set must NOT be called.
	c := newStubCache()
	sentinel := errors.New("db failure")

	fn := decorator.Func[string, product](func(_ context.Context, _ string) (product, error) {
		return product{}, sentinel
	})

	mw := decorator.WithCache[string, product](c, keyFn, time.Minute, zap.NewNop())
	wrapped := decorator.Chain(fn, mw)

	// Act
	_, err := wrapped(context.Background(), "4")

	// Assert
	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, 0, c.setCalls, "cache.Set must not be called when fn errors")
}
