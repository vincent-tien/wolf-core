// Package cache provides a pluggable caching abstraction for the wolf-be service.
package cache

import (
	"fmt"
	"time"

	"github.com/vincent-tien/wolf-core/infra/config"
)

// defaultLocalSize is used when LocalCacheConfig.Size is not set.
const defaultLocalSize = 10_000

// defaultLocalTTL is used when LocalCacheConfig.TTL is not set.
const defaultLocalTTL = 10 * time.Second

// NewClient constructs and returns the appropriate Client implementation based
// on the driver field:
//
//   - "redis" — returns a Redis-backed client (requires Redis to be reachable)
//   - any other value — returns a no-op client that silently discards writes
//     and returns ErrCacheMiss on reads
//
// When local.Enabled is true, the returned client is wrapped with an
// in-process L1 LRU cache to reduce remote round-trips for hot keys.
//
// An error is only returned for the "redis" driver when connectivity cannot
// be established during startup.
func NewClient(driver string, redis config.RedisConfig, local config.LocalCacheConfig) (Client, error) {
	var client Client

	switch driver {
	case config.CacheDriverRedis:
		rc, err := NewRedisClient(redis)
		if err != nil {
			return nil, fmt.Errorf("cache: factory: %w", err)
		}
		// Singleflight coalesces concurrent Gets for the same key,
		// preventing cache stampede on hot-key expiry.
		client = NewSingleflightCache(rc)
	default:
		client = NewNoopClient()
	}

	if local.Enabled {
		size := local.Size
		if size <= 0 {
			size = defaultLocalSize
		}
		ttl := local.TTL
		if ttl <= 0 {
			ttl = defaultLocalTTL
		}
		client = NewLocalCache(client, size, ttl)
	}

	return client, nil
}
