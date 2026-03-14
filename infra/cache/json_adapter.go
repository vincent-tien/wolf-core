// json_adapter.go — Generic typed JSON adapter over cache.Client.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// JSONAdapter bridges cache.Client (raw bytes) to the typed CacheGetter
// contract used by cqrs.WithQueryCaching. It serializes values as JSON.
type JSONAdapter[R any] struct {
	client Client
	prefix string
}

// NewJSONAdapter creates a typed cache adapter with a key prefix.
func NewJSONAdapter[R any](client Client, prefix string) *JSONAdapter[R] {
	return &JSONAdapter[R]{client: client, prefix: prefix}
}

// Get retrieves and deserializes a cached value. Returns (zero, false, nil) on
// cache miss — never propagates ErrCacheMiss as an error.
func (a *JSONAdapter[R]) Get(ctx context.Context, key string) (R, bool, error) {
	var zero R
	data, err := a.client.Get(ctx, a.prefix+key)
	if err != nil {
		if errors.Is(err, ErrCacheMiss) {
			return zero, false, nil
		}
		return zero, false, err
	}

	var result R
	if err := json.Unmarshal(data, &result); err != nil {
		_ = a.client.Delete(ctx, a.prefix+key)
		return zero, false, nil
	}

	return result, true, nil
}

// Set serializes and stores a value with the given TTL.
func (a *JSONAdapter[R]) Set(ctx context.Context, key string, value R, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: marshal value for key %q: %w", a.prefix+key, err)
	}
	return a.client.Set(ctx, a.prefix+key, data, ttl)
}

// Delete removes cached entries by key(s).
func (a *JSONAdapter[R]) Delete(ctx context.Context, keys ...string) error {
	prefixed := make([]string, len(keys))
	for i, k := range keys {
		prefixed[i] = a.prefix + k
	}
	return a.client.Delete(ctx, prefixed...)
}
