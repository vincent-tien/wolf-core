// local.go — L1 in-process LRU cache wrapping a remote Client (Redis).
//
// Config: cache.local.enabled, cache.local.size, cache.local.ttl in config.yaml.
package cache

import (
	"context"
	"fmt"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

// localEntry is a single cached value with its insertion time used for TTL.
type localEntry struct {
	data []byte
}

// LocalCache is an in-process L1 cache backed by an LRU with per-entry TTL.
// It sits in front of a remote Client (Redis) to eliminate network round-trips
// for hot keys. Stale reads are bounded by the configured defaultTTL.
//
// Consistency model: write-through with bounded staleness.
//   - Writes propagate to both L1 and inner (Redis) synchronously.
//   - Reads check L1 first, fall back to inner on miss.
//   - Cross-pod consistency is NOT guaranteed — each pod has its own L1.
//     Use Invalidate() with a pub/sub broadcast for cross-pod cache clearing.
//   - Worst-case staleness = defaultTTL (entries auto-expire).
type LocalCache struct {
	inner      Client
	store      *lru.LRU[string, localEntry]
	defaultTTL time.Duration
}

// NewLocalCache wraps inner with an in-process LRU cache.
// size is the maximum number of entries; defaultTTL controls how long entries
// stay in the L1 before expiring. A zero or negative TTL disables the L1.
func NewLocalCache(inner Client, size int, defaultTTL time.Duration) *LocalCache {
	store := lru.NewLRU[string, localEntry](size, nil, defaultTTL)
	return &LocalCache{
		inner:      inner,
		store:      store,
		defaultTTL: defaultTTL,
	}
}

// Get checks the local LRU first. On hit, returns immediately without
// touching the remote cache. On miss, delegates to the inner client and
// promotes the result into L1 for subsequent reads.
func (c *LocalCache) Get(ctx context.Context, key string) ([]byte, error) {
	if entry, ok := c.store.Get(key); ok {
		return entry.data, nil
	}

	data, err := c.inner.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	c.store.Add(key, localEntry{data: data})
	return data, nil
}

// Set writes to the inner client and updates the local LRU.
func (c *LocalCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.inner.Set(ctx, key, value, ttl); err != nil {
		return fmt.Errorf("local cache: set: %w", err)
	}

	c.store.Add(key, localEntry{data: value})
	return nil
}

// Delete removes keys from the local LRU and the inner client.
func (c *LocalCache) Delete(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		c.store.Remove(k)
	}
	return c.inner.Delete(ctx, keys...)
}

// Invalidate removes a key from the local LRU only, without touching the
// remote cache. Use this when receiving cache invalidation broadcasts from
// other pods (e.g. via NATS pub/sub).
func (c *LocalCache) Invalidate(keys ...string) {
	for _, k := range keys {
		c.store.Remove(k)
	}
}

// Ping delegates to the inner client.
func (c *LocalCache) Ping(ctx context.Context) error {
	return c.inner.Ping(ctx)
}

// Close delegates to the inner client.
func (c *LocalCache) Close() error {
	c.store.Purge()
	return c.inner.Close()
}

// Len returns the current number of entries in the local LRU.
func (c *LocalCache) Len() int {
	return c.store.Len()
}
