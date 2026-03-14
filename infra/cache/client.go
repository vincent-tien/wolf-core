// Package cache provides a pluggable caching abstraction for the wolf-be service.
// Backends are selected at startup via CacheConfig.Driver; the Client interface
// allows seamless swapping between Redis, an in-process noop, and future drivers.
package cache

import (
	"context"
	"errors"
	"time"
)

// ErrCacheMiss is returned by Client.Get when the requested key does not exist
// in the cache. Callers should treat this as a non-fatal, expected condition.
var ErrCacheMiss = errors.New("cache: miss")

// Client is the unified interface for all cache backends. Implementations must
// be safe for concurrent use from multiple goroutines.
type Client interface {
	// Get retrieves the raw byte value associated with key. If the key does
	// not exist ErrCacheMiss is returned. All other errors indicate a
	// backend communication failure.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores value under key with the given TTL. A TTL of 0 means the
	// entry never expires (behaviour is backend-specific).
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes one or more keys from the cache. Missing keys are
	// silently ignored.
	Delete(ctx context.Context, keys ...string) error

	// Ping checks that the cache backend is reachable. It is suitable for
	// use in health-check handlers.
	Ping(ctx context.Context) error

	// Close releases any resources held by the client. It must be called
	// exactly once when the application shuts down.
	Close() error
}
