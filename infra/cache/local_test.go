package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalCache_HitBypassesInner(t *testing.T) {
	inner := &countingClient{Client: NewNoopClient()}
	lc := NewLocalCache(inner, 100, 5*time.Second)

	ctx := context.Background()

	// Pre-populate via Set.
	require.NoError(t, lc.Set(ctx, "k1", []byte("v1"), time.Minute))
	assert.Equal(t, 1, inner.sets)

	// Get should hit L1, not inner.
	val, err := lc.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, []byte("v1"), val)
	assert.Equal(t, 0, inner.gets, "L1 hit should not call inner.Get")
}

func TestLocalCache_MissFallsThrough(t *testing.T) {
	inner := NewNoopClient() // always returns ErrCacheMiss
	lc := NewLocalCache(inner, 100, 5*time.Second)

	ctx := context.Background()
	_, err := lc.Get(ctx, "missing")
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestLocalCache_DeleteRemovesFromL1(t *testing.T) {
	inner := NewNoopClient()
	lc := NewLocalCache(inner, 100, 5*time.Second)

	ctx := context.Background()
	require.NoError(t, lc.Set(ctx, "k1", []byte("v1"), time.Minute))
	assert.Equal(t, 1, lc.Len())

	require.NoError(t, lc.Delete(ctx, "k1"))
	assert.Equal(t, 0, lc.Len())
}

func TestLocalCache_InvalidateLocalOnly(t *testing.T) {
	inner := &countingClient{Client: NewNoopClient()}
	lc := NewLocalCache(inner, 100, 5*time.Second)

	ctx := context.Background()
	require.NoError(t, lc.Set(ctx, "k1", []byte("v1"), time.Minute))

	lc.Invalidate("k1")
	assert.Equal(t, 0, lc.Len())
	assert.Equal(t, 0, inner.deletes, "Invalidate should not call inner.Delete")
}

func TestLocalCache_Close(t *testing.T) {
	inner := NewNoopClient()
	lc := NewLocalCache(inner, 100, 5*time.Second)

	ctx := context.Background()
	require.NoError(t, lc.Set(ctx, "k1", []byte("v1"), time.Minute))

	require.NoError(t, lc.Close())
	assert.Equal(t, 0, lc.Len())
}

// countingClient wraps a Client and counts method calls for assertions.
type countingClient struct {
	Client
	gets    int
	sets    int
	deletes int
}

func (c *countingClient) Get(ctx context.Context, key string) ([]byte, error) {
	c.gets++
	return c.Client.Get(ctx, key)
}

func (c *countingClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.sets++
	return c.Client.Set(ctx, key, value, ttl)
}

func (c *countingClient) Delete(ctx context.Context, keys ...string) error {
	c.deletes++
	return c.Client.Delete(ctx, keys...)
}
