// cache.go — CQRS query middleware that caches results via cache.Client.
package decorator

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/cache"
)

// WithCache returns a Middleware that implements the cache-aside pattern using
// the raw-byte cache.Client. Out values are serialized with encoding/json.
// Cache errors are non-fatal: they are logged at Warn level and execution
// falls through to fn. If fn returns an error the result is never cached.
// keyFn derives the cache key deterministically from the input.
func WithCache[In, Out any](
	client cache.Client,
	keyFn func(In) string,
	ttl time.Duration,
	logger *zap.Logger,
) Middleware[In, Out] {
	return func(next Func[In, Out]) Func[In, Out] {
		return func(ctx context.Context, in In) (Out, error) {
			key := keyFn(in)

			if raw, err := client.Get(ctx, key); err == nil {
				var cached Out
				if jsonErr := json.Unmarshal(raw, &cached); jsonErr == nil {
					return cached, nil
				}
			} else if !errors.Is(err, cache.ErrCacheMiss) {
				logger.Warn("cache get error", zap.String("key", key), zap.Error(err))
			}

			result, err := next(ctx, in)
			if err != nil {
				return result, err
			}

			raw, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				logger.Warn("cache marshal error", zap.String("key", key), zap.Error(marshalErr))
				return result, nil
			}

			if setErr := client.Set(ctx, key, raw, ttl); setErr != nil {
				logger.Warn("cache set error", zap.String("key", key), zap.Error(setErr))
			}

			return result, nil
		}
	}
}
