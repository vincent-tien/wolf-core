// standard_chain.go — One-liner convenience wrappers for the standard
// middleware ordering. Every command/query in every module should use these
// instead of manually calling ChainCommand/ChainQuery, ensuring consistent
// middleware ordering across the entire codebase.
//
// Standard execution order (outermost → innermost):
//   tracing → validation → logging → metrics → handler
// WithInvalidation variant:
//   tracing → cache invalidation → validation → logging → metrics → handler
// ChainCommand/ChainQuery iterate last-to-first, so the FIRST argument
// becomes the outermost wrapper. Tracing wraps everything so the span
// captures the full pipeline. Validation short-circuits before logging
// if input is invalid. Metrics wraps the handler (captures handler latency).
// Logging captures the handler result including duration.
package cqrs

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// ChainDeps holds the cross-cutting dependencies shared by all standard CQRS
// middleware chains. It accepts raw Prometheus types to avoid creating a
// dependency from shared/ to platform/ (layer violation).
type ChainDeps struct {
	Logger          *zap.Logger
	CommandDuration *prometheus.HistogramVec
	CommandTotal    *prometheus.CounterVec
	QueryDuration   *prometheus.HistogramVec
	QueryTotal      *prometheus.CounterVec
	CtxFields       CtxFieldsFunc
	Validate        func(any) error

	// QueryCacheHits/QueryCacheMisses are optional per-query cache observability
	// counters (label: query_name). When non-nil, StandardQueryChainWithCaching
	// wraps the cache with a metrics decorator that increments on every Get.
	QueryCacheHits   *prometheus.CounterVec
	QueryCacheMisses *prometheus.CounterVec
}

// StandardCommandChain wraps a CommandHandler with the standard middleware
// chain: tracing → validation → logging → metrics (outermost to innermost).
func StandardCommandChain[C Command, R any](handler CommandHandler[C, R], name string, deps ChainDeps) CommandHandler[C, R] {
	return ChainCommand(handler,
		WithCommandTracing[C, R](name),
		WithCommandValidation[C, R](func(c C) error { return deps.Validate(c) }),
		WithCommandLogging[C, R](deps.Logger, name, deps.CtxFields),
		WithCommandMetrics[C, R](deps.CommandDuration, deps.CommandTotal, name),
	)
}

// StandardCommandChainWithInvalidation wraps a CommandHandler with the standard
// middleware chain plus cache invalidation:
//
//	tracing → cache invalidation → validation → logging → metrics → handler
//
// Cache eviction fires only after a successful handler execution (validation
// failures short-circuit before the Delete call).
func StandardCommandChainWithInvalidation[C Command, R any](
	handler CommandHandler[C, R],
	name string,
	cache CacheInvalidator,
	keyFn func(C) string,
	deps ChainDeps,
) CommandHandler[C, R] {
	return ChainCommand(handler,
		WithCommandTracing[C, R](name),
		WithCommandCacheInvalidation[C, R](cache, keyFn, deps.Logger),
		WithCommandValidation[C, R](func(c C) error { return deps.Validate(c) }),
		WithCommandLogging[C, R](deps.Logger, name, deps.CtxFields),
		WithCommandMetrics[C, R](deps.CommandDuration, deps.CommandTotal, name),
	)
}

// StandardQueryChain wraps a QueryHandler with the standard middleware
// chain: tracing → validation → logging → metrics (outermost to innermost).
func StandardQueryChain[Q Query, R any](handler QueryHandler[Q, R], name string, deps ChainDeps) QueryHandler[Q, R] {
	return ChainQuery(handler,
		WithQueryTracing[Q, R](name),
		WithQueryValidation[Q, R](func(q Q) error { return deps.Validate(q) }),
		WithQueryLogging[Q, R](deps.Logger, name, deps.CtxFields),
		WithQueryMetrics[Q, R](deps.QueryDuration, deps.QueryTotal, name),
	)
}

// StandardQueryChainWithCaching wraps a QueryHandler with the standard
// middleware chain plus cache-aside:
//
//	tracing → caching → validation → logging → metrics → handler
//
// Tracing is outermost so the span captures cache hits as fast completions
// (giving visibility into cache hit ratios from traces). On a cache hit the
// downstream chain is bypassed. Cache errors are logged but never propagate.
//
// When deps.QueryCacheHits and deps.QueryCacheMisses are non-nil, the cache is
// wrapped with a metrics decorator that increments per-query hit/miss counters.
func StandardQueryChainWithCaching[Q Query, R any](
	handler QueryHandler[Q, R],
	name string,
	cache CacheGetter[R],
	keyFn func(Q) string,
	ttl time.Duration,
	deps ChainDeps,
) QueryHandler[Q, R] {
	c := cache
	if deps.QueryCacheHits != nil && deps.QueryCacheMisses != nil {
		c = &metricsCacheGetter[R]{
			inner:  cache,
			hits:   deps.QueryCacheHits.WithLabelValues(name),
			misses: deps.QueryCacheMisses.WithLabelValues(name),
		}
	}

	return ChainQuery(handler,
		WithQueryTracing[Q, R](name),
		WithQueryCaching[Q, R](c, keyFn, ttl, deps.Logger),
		WithQueryValidation[Q, R](func(q Q) error { return deps.Validate(q) }),
		WithQueryLogging[Q, R](deps.Logger, name, deps.CtxFields),
		WithQueryMetrics[Q, R](deps.QueryDuration, deps.QueryTotal, name),
	)
}

// metricsCacheGetter wraps a CacheGetter with Prometheus hit/miss counters.
type metricsCacheGetter[R any] struct {
	inner  CacheGetter[R]
	hits   prometheus.Counter
	misses prometheus.Counter
}

func (m *metricsCacheGetter[R]) Get(ctx context.Context, key string) (R, bool, error) {
	val, hit, err := m.inner.Get(ctx, key)
	if err == nil {
		if hit {
			m.hits.Inc()
		} else {
			m.misses.Inc()
		}
	}
	return val, hit, err
}

func (m *metricsCacheGetter[R]) Set(ctx context.Context, key string, value R, ttl time.Duration) error {
	return m.inner.Set(ctx, key, value, ttl)
}
