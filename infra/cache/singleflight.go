// singleflight.go — Cache stampede protection via x/sync/singleflight.
package cache

import (
	"context"
	"time"

	"golang.org/x/sync/singleflight"
)

// SingleflightCache wraps a cache.Client with singleflight to coalesce
// concurrent Gets for the same key, preventing cache stampede (thundering herd).
// Set, Delete, Ping, and Close delegate directly without coalescing.
type SingleflightCache struct {
	inner Client
	group singleflight.Group
}

// NewSingleflightCache returns a Client that coalesces concurrent Get calls
// for the same key through the provided inner Client.
func NewSingleflightCache(inner Client) *SingleflightCache {
	return &SingleflightCache{inner: inner}
}

// Get retrieves the value for key, coalescing concurrent calls so that only one
// request to the inner client is made per key at any given time.
// Returns ErrCacheMiss when the inner client returns nil data (consistent with
// the Client contract).
func (c *SingleflightCache) Get(ctx context.Context, key string) ([]byte, error) {
	v, err, _ := c.group.Do(key, func() (any, error) {
		return c.inner.Get(ctx, key)
	})
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrCacheMiss
	}
	return v.([]byte), nil
}

// Set delegates directly to the inner client (no coalescing needed for writes).
func (c *SingleflightCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.inner.Set(ctx, key, value, ttl)
}

// Delete delegates directly to the inner client.
func (c *SingleflightCache) Delete(ctx context.Context, keys ...string) error {
	return c.inner.Delete(ctx, keys...)
}

// Ping delegates directly to the inner client.
func (c *SingleflightCache) Ping(ctx context.Context) error {
	return c.inner.Ping(ctx)
}

// Close delegates directly to the inner client.
func (c *SingleflightCache) Close() error {
	return c.inner.Close()
}
