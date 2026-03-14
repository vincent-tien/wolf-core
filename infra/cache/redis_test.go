// Package cache provides a pluggable caching abstraction for the wolf-be service.
package cache_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/cache"
	"github.com/vincent-tien/wolf-core/infra/config"
)

// redisAddr returns the Redis address to use in tests. It prefers the
// CACHE_REDIS_ADDR environment variable so CI can point at a real container.
func redisAddr() string {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		return addr
	}

	return "localhost:6379"
}

// newTestRedisClient creates a Redis client for integration tests and skips
// the test if Redis is not reachable.
func newTestRedisClient(t *testing.T) cache.Client {
	t.Helper()

	cfg := config.RedisConfig{
		Addr:         redisAddr(),
		Password:     "",
		DB:           15, // use a dedicated DB to avoid polluting other data
		PoolSize:     5,
		MinIdleConns: 1,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	}

	client, err := cache.NewRedisClient(cfg)
	if err != nil {
		t.Skipf("skipping redis integration test: %v", err)
	}

	t.Cleanup(func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Logf("warning: redis client close: %v", closeErr)
		}
	})

	return client
}

func TestRedisClient_Ping(t *testing.T) {
	client := newTestRedisClient(t)

	ctx := context.Background()

	err := client.Ping(ctx)

	assert.NoError(t, err)
}

func TestRedisClient_SetAndGet(t *testing.T) {
	client := newTestRedisClient(t)

	ctx := context.Background()
	key := "wolf:test:set-get"
	value := []byte("hello-wolf")

	// Arrange: ensure the key does not exist beforehand.
	require.NoError(t, client.Delete(ctx, key))

	// Act: store the value.
	require.NoError(t, client.Set(ctx, key, value, 30*time.Second))

	// Assert: retrieve and compare.
	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, got)

	// Cleanup.
	require.NoError(t, client.Delete(ctx, key))
}

func TestRedisClient_Get_MissingKey(t *testing.T) {
	client := newTestRedisClient(t)

	ctx := context.Background()
	key := "wolf:test:does-not-exist"

	// Arrange: ensure the key really is absent.
	require.NoError(t, client.Delete(ctx, key))

	// Act.
	_, err := client.Get(ctx, key)

	// Assert: must be ErrCacheMiss, not a connectivity error.
	assert.True(t, errors.Is(err, cache.ErrCacheMiss), "expected ErrCacheMiss, got %v", err)
}

func TestRedisClient_Delete_SingleKey(t *testing.T) {
	client := newTestRedisClient(t)

	ctx := context.Background()
	key := "wolf:test:delete-single"

	require.NoError(t, client.Set(ctx, key, []byte("bye"), 30*time.Second))

	// Act.
	require.NoError(t, client.Delete(ctx, key))

	// Assert: key is gone.
	_, err := client.Get(ctx, key)
	assert.True(t, errors.Is(err, cache.ErrCacheMiss))
}

func TestRedisClient_Delete_MultipleKeys(t *testing.T) {
	client := newTestRedisClient(t)

	ctx := context.Background()
	keys := []string{"wolf:test:multi-del-1", "wolf:test:multi-del-2"}

	for _, k := range keys {
		require.NoError(t, client.Set(ctx, k, []byte("v"), 30*time.Second))
	}

	// Act: delete all at once.
	require.NoError(t, client.Delete(ctx, keys...))

	// Assert: both keys are gone.
	for _, k := range keys {
		_, err := client.Get(ctx, k)
		assert.True(t, errors.Is(err, cache.ErrCacheMiss), "key %q should be missing", k)
	}
}

func TestRedisClient_Delete_MissingKey_IsNoOp(t *testing.T) {
	client := newTestRedisClient(t)

	ctx := context.Background()
	key := "wolf:test:delete-missing"

	// Arrange: ensure the key does not exist.
	require.NoError(t, client.Delete(ctx, key))

	// Act: deleting a missing key must not error.
	err := client.Delete(ctx, key)

	assert.NoError(t, err)
}

func TestRedisClient_Set_TTLExpiry(t *testing.T) {
	client := newTestRedisClient(t)

	ctx := context.Background()
	key := "wolf:test:ttl-expiry"
	ttl := 100 * time.Millisecond

	require.NoError(t, client.Set(ctx, key, []byte("expires-soon"), ttl))

	// Wait for expiry with a small buffer.
	time.Sleep(ttl + 150*time.Millisecond)

	_, err := client.Get(ctx, key)
	assert.True(t, errors.Is(err, cache.ErrCacheMiss), "key should have expired")
}
