// Package cache provides a pluggable caching abstraction for the wolf-be service.
package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/vincent-tien/wolf-core/infra/config"
)

// redisClient wraps a go-redis/v9 client and implements the Client interface.
type redisClient struct {
	rdb *goredis.Client
}

// NewRedisClient constructs a Redis-backed Client from the supplied RedisConfig,
// verifies connectivity with a Ping, and returns the client ready for use.
// The caller is responsible for calling Close when the client is no longer needed.
func NewRedisClient(cfg config.RedisConfig) (Client, error) {
	rdb := goredis.NewClient(&goredis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		DialTimeout:  cfg.DialTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		// Release the underlying connection pool before returning.
		if closeErr := rdb.Close(); closeErr != nil {
			return nil, fmt.Errorf("cache: redis ping failed (%w); also failed to close client: %v", err, closeErr)
		}

		return nil, fmt.Errorf("cache: redis ping: %w", err)
	}

	return &redisClient{rdb: rdb}, nil
}

// Get retrieves the raw byte value for key. Returns ErrCacheMiss when the key
// is absent, or a wrapped error for any other backend failure.
func (c *redisClient) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, ErrCacheMiss
		}

		return nil, fmt.Errorf("cache: redis get %q: %w", key, err)
	}

	return val, nil
}

// Set stores value under key with the given TTL. A TTL of 0 means the entry
// persists until explicitly deleted or the Redis instance restarts (no expiry).
func (c *redisClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("cache: redis set %q: %w", key, err)
	}

	return nil
}

// Delete removes one or more keys. Missing keys are silently ignored by Redis.
func (c *redisClient) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache: redis delete: %w", err)
	}

	return nil
}

// Ping issues a Redis PING command to verify backend reachability.
func (c *redisClient) Ping(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cache: redis ping: %w", err)
	}

	return nil
}

// Close gracefully shuts down the connection pool.
func (c *redisClient) Close() error {
	if err := c.rdb.Close(); err != nil {
		return fmt.Errorf("cache: redis close: %w", err)
	}

	return nil
}
