// blacklist_redis.go — Redis-backed token blacklist for production logout/revocation.
package auth

import (
	"context"
	"errors"
	"time"

	"github.com/vincent-tien/wolf-core/infra/cache"
)

// RedisBlacklist stores revoked JTIs and session identifiers in Redis.
// Each entry lives until the corresponding token's natural expiry so that
// the blacklist stays compact without manual cleanup.
//
// Key format: "bl:{jti}"
type RedisBlacklist struct {
	cache cache.Client
}

// NewRedisBlacklist creates a RedisBlacklist backed by the given cache client.
func NewRedisBlacklist(c cache.Client) *RedisBlacklist {
	return &RedisBlacklist{cache: c}
}

// IsBlacklisted returns true if the given JTI has been blacklisted.
// A cache miss is treated as "not blacklisted" and is not an error.
func (b *RedisBlacklist) IsBlacklisted(ctx context.Context, jti string) (bool, error) {
	_, err := b.cache.Get(ctx, blacklistKey(jti))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, cache.ErrCacheMiss) {
		return false, nil
	}
	return false, err
}

// Blacklist adds the given JTI to the blacklist with a TTL equal to the
// remaining lifetime of the token. If the token has already expired
// (ttl <= 0) the entry is skipped because no further validation would
// succeed anyway.
func (b *RedisBlacklist) Blacklist(ctx context.Context, jti string, expiresAt time.Time) error {
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil
	}
	return b.cache.Set(ctx, blacklistKey(jti), []byte("1"), ttl)
}

// blacklistKey returns the Redis key for a given JTI or session identifier.
func blacklistKey(jti string) string {
	return "bl:" + jti
}
