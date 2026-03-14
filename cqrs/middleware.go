// Package cqrs provides middleware factories for CommandHandler and QueryHandler.
// Middlewares implement cross-cutting concerns such as logging, validation,
// metrics, tracing, and caching without polluting use-case business logic.
package cqrs

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const tracerName = "wolf-core/cqrs"

// ---------------------------------------------------------------------------
// Tracing middlewares
// ---------------------------------------------------------------------------

// WithCommandTracing returns a CommandMiddleware that creates a child span
// for the command execution. Place outermost in the chain so the span
// captures the full middleware pipeline (validation, logging, metrics).
// Tracer, span name, and attributes are pre-computed at construction time
// to avoid per-call mutex contention on the OTel TracerProvider.
func WithCommandTracing[C Command, R any](commandName string) CommandMiddleware[C, R] {
	tracer := otel.Tracer(tracerName)
	spanName := "command/" + commandName
	spanOpts := []trace.SpanStartOption{
		trace.WithAttributes(attribute.String("cqrs.type", "command"), attribute.String("cqrs.name", commandName)),
		trace.WithSpanKind(trace.SpanKindInternal),
	}

	return func(next CommandHandler[C, R]) CommandHandler[C, R] {
		return CommandHandlerFunc[C, R](func(ctx context.Context, cmd C) (R, error) {
			ctx, span := tracer.Start(ctx, spanName, spanOpts...)
			defer span.End()

			result, err := next.Handle(ctx, cmd)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			return result, err
		})
	}
}

// WithQueryTracing returns a QueryMiddleware that creates a child span
// for the query execution. Tracer and attributes are pre-computed at
// construction time to avoid per-call overhead.
func WithQueryTracing[Q Query, R any](queryName string) QueryMiddleware[Q, R] {
	tracer := otel.Tracer(tracerName)
	spanName := "query/" + queryName
	spanOpts := []trace.SpanStartOption{
		trace.WithAttributes(attribute.String("cqrs.type", "query"), attribute.String("cqrs.name", queryName)),
		trace.WithSpanKind(trace.SpanKindInternal),
	}

	return func(next QueryHandler[Q, R]) QueryHandler[Q, R] {
		return QueryHandlerFunc[Q, R](func(ctx context.Context, query Q) (R, error) {
			ctx, span := tracer.Start(ctx, spanName, spanOpts...)
			defer span.End()

			result, err := next.Handle(ctx, query)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			return result, err
		})
	}
}

// ---------------------------------------------------------------------------
// Command middlewares
// ---------------------------------------------------------------------------

// CtxFieldsFunc extracts observability fields (request_id, trace_id, etc.)
// from context for log enrichment. Defined here so shared/cqrs stays
// infra-free; callers inject the concrete extractor at wiring time.
type CtxFieldsFunc func(context.Context) []zap.Field

// WithCommandLogging returns a CommandMiddleware that logs the command name,
// execution duration, and any error at Info level using the supplied zap.Logger.
// An optional ctxFields extractor enriches every log line with request-scoped
// observability fields (request_id, trace_id, correlation_id, user_id).
func WithCommandLogging[C Command, R any](logger *zap.Logger, commandName string, ctxFields ...CtxFieldsFunc) CommandMiddleware[C, R] {
	return func(next CommandHandler[C, R]) CommandHandler[C, R] {
		return CommandHandlerFunc[C, R](func(ctx context.Context, cmd C) (R, error) {
			start := time.Now()

			result, err := next.Handle(ctx, cmd)

			fields := make([]zap.Field, 0, 8)
			for _, fn := range ctxFields {
				fields = append(fields, fn(ctx)...)
			}
			fields = append(fields,
				zap.String("command", commandName),
				zap.Duration("duration", time.Since(start)),
			)
			if err != nil {
				fields = append(fields, zap.Error(err))
			}

			logger.Info("command handled", fields...)

			return result, err
		})
	}
}

// WithCommandValidation returns a CommandMiddleware that calls validateFn
// before delegating to the next handler. If validateFn returns a non-nil
// error the handler is short-circuited and the zero value of R is returned.
func WithCommandValidation[C Command, R any](validateFn func(C) error) CommandMiddleware[C, R] {
	return func(next CommandHandler[C, R]) CommandHandler[C, R] {
		return CommandHandlerFunc[C, R](func(ctx context.Context, cmd C) (R, error) {
			var zero R

			if err := validateFn(cmd); err != nil {
				return zero, err
			}

			return next.Handle(ctx, cmd)
		})
	}
}

// WithCommandMetrics returns a CommandMiddleware that records execution
// duration and increments an invocation counter using the provided
// Prometheus collectors. Labels recorded are: command_name and status
// ("success" or "error").
func WithCommandMetrics[C Command, R any](
	duration *prometheus.HistogramVec,
	total *prometheus.CounterVec,
	commandName string,
) CommandMiddleware[C, R] {
	return func(next CommandHandler[C, R]) CommandHandler[C, R] {
		return CommandHandlerFunc[C, R](func(ctx context.Context, cmd C) (R, error) {
			start := time.Now()

			result, err := next.Handle(ctx, cmd)

			status := "success"
			if err != nil {
				status = "error"
			}

			duration.WithLabelValues(commandName, status).Observe(time.Since(start).Seconds())
			total.WithLabelValues(commandName, status).Inc()

			return result, err
		})
	}
}

// ---------------------------------------------------------------------------
// Query middlewares
// ---------------------------------------------------------------------------

// WithQueryLogging returns a QueryMiddleware that logs the query name,
// execution duration, and any error at Info level using the supplied zap.Logger.
// An optional ctxFields extractor enriches every log line with request-scoped
// observability fields.
func WithQueryLogging[Q Query, R any](logger *zap.Logger, queryName string, ctxFields ...CtxFieldsFunc) QueryMiddleware[Q, R] {
	return func(next QueryHandler[Q, R]) QueryHandler[Q, R] {
		return QueryHandlerFunc[Q, R](func(ctx context.Context, query Q) (R, error) {
			start := time.Now()

			result, err := next.Handle(ctx, query)

			fields := make([]zap.Field, 0, 8)
			for _, fn := range ctxFields {
				fields = append(fields, fn(ctx)...)
			}
			fields = append(fields,
				zap.String("query", queryName),
				zap.Duration("duration", time.Since(start)),
			)
			if err != nil {
				fields = append(fields, zap.Error(err))
			}

			logger.Info("query handled", fields...)

			return result, err
		})
	}
}

// WithQueryValidation returns a QueryMiddleware that calls validateFn
// before delegating to the next handler. If validateFn returns a non-nil
// error the handler is short-circuited and the zero value of R is returned.
func WithQueryValidation[Q Query, R any](validateFn func(Q) error) QueryMiddleware[Q, R] {
	return func(next QueryHandler[Q, R]) QueryHandler[Q, R] {
		return QueryHandlerFunc[Q, R](func(ctx context.Context, query Q) (R, error) {
			var zero R

			if err := validateFn(query); err != nil {
				return zero, err
			}

			return next.Handle(ctx, query)
		})
	}
}

// WithQueryMetrics returns a QueryMiddleware that records execution
// duration and increments an invocation counter using the provided
// Prometheus collectors. Labels recorded are: query_name and status
// ("success" or "error").
func WithQueryMetrics[Q Query, R any](
	duration *prometheus.HistogramVec,
	total *prometheus.CounterVec,
	queryName string,
) QueryMiddleware[Q, R] {
	return func(next QueryHandler[Q, R]) QueryHandler[Q, R] {
		return QueryHandlerFunc[Q, R](func(ctx context.Context, query Q) (R, error) {
			start := time.Now()

			result, err := next.Handle(ctx, query)

			status := "success"
			if err != nil {
				status = "error"
			}

			duration.WithLabelValues(queryName, status).Observe(time.Since(start).Seconds())
			total.WithLabelValues(queryName, status).Inc()

			return result, err
		})
	}
}

// CacheGetter is the read/write contract required by WithQueryCaching.
// Implementations must be safe for concurrent use.
type CacheGetter[R any] interface {
	// Get retrieves a cached value by key. The second return value indicates
	// whether the key was present (true = hit, false = miss).
	Get(ctx context.Context, key string) (R, bool, error)

	// Set stores a value under key with the given TTL.
	Set(ctx context.Context, key string, value R, ttl time.Duration) error
}

// CacheInvalidator is the delete-only contract required by WithCommandCacheInvalidation.
type CacheInvalidator interface {
	Delete(ctx context.Context, keys ...string) error
}

// WithCommandCacheInvalidation returns a CommandMiddleware that evicts a cache
// key after a successful command execution. keyFn extracts the cache key from
// the command. Cache eviction failures are logged but never propagate — the
// command already committed successfully and stale reads self-heal via TTL.
func WithCommandCacheInvalidation[C Command, R any](
	cache CacheInvalidator,
	keyFn func(C) string,
	logger *zap.Logger,
) CommandMiddleware[C, R] {
	return func(next CommandHandler[C, R]) CommandHandler[C, R] {
		return CommandHandlerFunc[C, R](func(ctx context.Context, cmd C) (R, error) {
			result, err := next.Handle(ctx, cmd)
			if err != nil {
				return result, err
			}

			key := keyFn(cmd)
			if delErr := cache.Delete(ctx, key); delErr != nil {
				logger.Warn("cache invalidation error", zap.String("key", key), zap.Error(delErr))
			}

			return result, nil
		})
	}
}

// WithQueryCaching returns a QueryMiddleware that wraps the handler with a
// cache-aside pattern. On a cache hit the handler is bypassed entirely. On a
// miss the handler result is stored in the cache before being returned.
// Cache errors are logged at Warn level but never propagate to the caller.
func WithQueryCaching[Q Query, R any](
	cache CacheGetter[R],
	keyFn func(Q) string,
	ttl time.Duration,
	logger *zap.Logger,
) QueryMiddleware[Q, R] {
	return func(next QueryHandler[Q, R]) QueryHandler[Q, R] {
		return QueryHandlerFunc[Q, R](func(ctx context.Context, query Q) (R, error) {
			key := keyFn(query)

			cached, hit, err := cache.Get(ctx, key)
			if err != nil {
				logger.Warn("cache get error", zap.String("key", key), zap.Error(err))
			} else if hit {
				return cached, nil
			}

			result, err := next.Handle(ctx, query)
			if err != nil {
				return result, err
			}

			if setErr := cache.Set(ctx, key, result, ttl); setErr != nil {
				logger.Warn("cache set error", zap.String("key", key), zap.Error(setErr))
			}

			return result, nil
		})
	}
}
