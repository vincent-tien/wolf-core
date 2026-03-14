package cache_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/cache"
)

// spyClient is a test double that counts Get invocations and allows
// configuring the response or error returned.
type spyClient struct {
	getCalls atomic.Int64
	getDelay time.Duration
	getVal   []byte
	getErr   error
	setCalls atomic.Int64
}

func (s *spyClient) Get(ctx context.Context, key string) ([]byte, error) {
	s.getCalls.Add(1)
	if s.getDelay > 0 {
		time.Sleep(s.getDelay)
	}
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.getVal == nil {
		return nil, cache.ErrCacheMiss
	}
	return s.getVal, nil
}

func (s *spyClient) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	s.setCalls.Add(1)
	return nil
}

func (s *spyClient) Delete(_ context.Context, _ ...string) error { return nil }
func (s *spyClient) Ping(_ context.Context) error                { return nil }
func (s *spyClient) Close() error                                { return nil }

func TestSingleflightCache_CoalescesConcurrentGets(t *testing.T) {
	// Arrange
	inner := &spyClient{
		getVal:   []byte("cached-value"),
		getDelay: 50 * time.Millisecond,
	}
	sf := cache.NewSingleflightCache(inner)

	const concurrency = 100
	ctx := context.Background()

	// Act: fire 100 concurrent Gets for the same key.
	var wg sync.WaitGroup
	results := make([][]byte, concurrency)
	errs := make([]error, concurrency)

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = sf.Get(ctx, "same-key")
		}(i)
	}
	wg.Wait()

	// Assert: inner should have been called exactly once.
	assert.Equal(t, int64(1), inner.getCalls.Load(), "expected 1 call to inner, got %d", inner.getCalls.Load())
	for i := range concurrency {
		require.NoError(t, errs[i])
		assert.Equal(t, []byte("cached-value"), results[i])
	}
}

func TestSingleflightCache_DifferentKeysNotCoalesced(t *testing.T) {
	// Arrange
	inner := &spyClient{
		getVal:   []byte("v"),
		getDelay: 20 * time.Millisecond,
	}
	sf := cache.NewSingleflightCache(inner)
	ctx := context.Background()

	// Act: concurrent Gets for different keys.
	var wg sync.WaitGroup
	keys := []string{"key-a", "key-b", "key-c"}
	for _, k := range keys {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, _ = sf.Get(ctx, key)
		}(k)
	}
	wg.Wait()

	// Assert: each key triggers its own call.
	assert.Equal(t, int64(len(keys)), inner.getCalls.Load())
}

func TestSingleflightCache_SetDelegatesDirectly(t *testing.T) {
	// Arrange
	inner := &spyClient{}
	sf := cache.NewSingleflightCache(inner)
	ctx := context.Background()

	// Act
	err := sf.Set(ctx, "key", []byte("val"), time.Minute)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, int64(1), inner.setCalls.Load())
}

func TestSingleflightCache_ErrorPropagated(t *testing.T) {
	// Arrange
	errBackend := errors.New("redis: connection refused")
	inner := &spyClient{
		getErr:   errBackend,
		getDelay: 20 * time.Millisecond,
	}
	sf := cache.NewSingleflightCache(inner)

	const concurrency = 10
	ctx := context.Background()

	// Act
	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = sf.Get(ctx, "fail-key")
		}(i)
	}
	wg.Wait()

	// Assert: all waiters receive the same error.
	for i := range concurrency {
		assert.ErrorIs(t, errs[i], errBackend)
	}
	assert.Equal(t, int64(1), inner.getCalls.Load())
}

func TestSingleflightCache_CacheMissPropagated(t *testing.T) {
	// Arrange: nil getVal means the spy returns ErrCacheMiss.
	inner := &spyClient{}
	sf := cache.NewSingleflightCache(inner)
	ctx := context.Background()

	// Act
	_, err := sf.Get(ctx, "missing-key")

	// Assert
	assert.ErrorIs(t, err, cache.ErrCacheMiss)
}
