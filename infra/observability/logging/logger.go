// Package logging provides structured logging via zap.
// It exposes a factory function that builds a *zap.Logger configured for the
// given level and format, and helpers to attach/extract per-request context
// fields (request_id, trace_id, correlation_id, user_id).
package logging

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ctxKey is an unexported type used as context key to avoid collisions.
type ctxKey string

const (
	keyRequestID     ctxKey = "request_id"
	keyTraceID       ctxKey = "trace_id"
	keyCorrelationID ctxKey = "correlation_id"
	keyUserID        ctxKey = "user_id"
)

// New builds and returns a *zap.Logger configured for the given parameters.
// format must be "json" (production) or "console" (development).
// level must be one of "debug", "info", "warn", or "error"; defaults to "info".
// The returned logger carries two permanent fields: "service" and "env".
func New(level, format, serviceName, env string) (*zap.Logger, error) {
	zapLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	var base *zap.Logger
	if format == "json" {
		cfg := zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(zapLevel)
		base, err = cfg.Build()
	} else {
		cfg := zap.NewDevelopmentConfig()
		cfg.Level = zap.NewAtomicLevelAt(zapLevel)
		base, err = cfg.Build()
	}
	if err != nil {
		return nil, err
	}

	return base.With(
		zap.String("service", serviceName),
		zap.String("env", env),
	), nil
}

// ContextFields extracts well-known observability fields from ctx and returns
// them as a slice of zap.Field values ready to be passed to a logger call.
// Fields are omitted when their corresponding value is absent from ctx.
func ContextFields(ctx context.Context) []zap.Field {
	fields := make([]zap.Field, 0, 4)

	if v := RequestIDFromContext(ctx); v != "" {
		fields = append(fields, zap.String(string(keyRequestID), v))
	}

	// Try custom context key first, fall back to OTel span for trace_id.
	if v := TraceIDFromContext(ctx); v != "" {
		fields = append(fields, zap.String(string(keyTraceID), v))
	} else if v := OTelTraceIDFromContext(ctx); v != "" {
		fields = append(fields, zap.String(string(keyTraceID), v))
	}

	if v := CorrelationIDFromContext(ctx); v != "" {
		fields = append(fields, zap.String(string(keyCorrelationID), v))
	}
	if v := UserIDFromContext(ctx); v != "" {
		fields = append(fields, zap.String(string(keyUserID), v))
	}

	return fields
}

// --------------------------------------------------------------------------
// RequestID helpers
// --------------------------------------------------------------------------

// WithRequestID returns a new context that carries the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestIDFromContext returns the request ID stored in ctx, or an empty string
// when none is present.
func RequestIDFromContext(ctx context.Context) string {
	return stringFromCtx(ctx, keyRequestID)
}

// --------------------------------------------------------------------------
// TraceID helpers
// --------------------------------------------------------------------------

// WithTraceID returns a new context that carries the given trace ID.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyTraceID, id)
}

// TraceIDFromContext returns the trace ID stored in ctx, or an empty string
// when none is present.
func TraceIDFromContext(ctx context.Context) string {
	return stringFromCtx(ctx, keyTraceID)
}

// OTelTraceIDFromContext extracts the trace ID directly from the OTel span
// context, returning an empty string if no active span exists.
func OTelTraceIDFromContext(ctx context.Context) string {
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}

// --------------------------------------------------------------------------
// CorrelationID helpers
// --------------------------------------------------------------------------

// WithCorrelationID returns a new context that carries the given correlation ID.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyCorrelationID, id)
}

// CorrelationIDFromContext returns the correlation ID stored in ctx, or an
// empty string when none is present.
func CorrelationIDFromContext(ctx context.Context) string {
	return stringFromCtx(ctx, keyCorrelationID)
}

// --------------------------------------------------------------------------
// UserID helpers
// --------------------------------------------------------------------------

// WithUserID returns a new context that carries the given user ID.
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyUserID, id)
}

// UserIDFromContext returns the user ID stored in ctx, or an empty string
// when none is present.
func UserIDFromContext(ctx context.Context) string {
	return stringFromCtx(ctx, keyUserID)
}

// --------------------------------------------------------------------------
// Trace context helpers
// --------------------------------------------------------------------------

// WithTraceContext extracts the active OpenTelemetry trace_id and span_id from
// ctx and returns a child logger enriched with those fields. If ctx does not
// contain a valid span, the original logger is returned unchanged.
func WithTraceContext(ctx context.Context, logger *zap.Logger) *zap.Logger {
	span := trace.SpanFromContext(ctx)
	sc := span.SpanContext()
	if !sc.IsValid() {
		return logger
	}
	return logger.With(
		zap.String("trace_id", sc.TraceID().String()),
		zap.String("span_id", sc.SpanID().String()),
	)
}

// --------------------------------------------------------------------------
// internal helpers
// --------------------------------------------------------------------------

// stringFromCtx safely retrieves a string value from ctx by key.
func stringFromCtx(ctx context.Context, key ctxKey) string {
	if v, ok := ctx.Value(key).(string); ok {
		return v
	}
	return ""
}

// parseLevel maps a string log level to a zapcore.Level.
// Returns zapcore.InfoLevel and a nil error for unknown strings so callers
// get a sensible default without failing at startup.
func parseLevel(level string) (zapcore.Level, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		// Default to info rather than erroring on unknown values.
		return zapcore.InfoLevel, nil
	}
	return zapLevel, nil
}
