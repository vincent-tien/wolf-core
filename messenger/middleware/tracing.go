// tracing.go — Messenger middleware creating OpenTelemetry spans for dispatch.
package middleware

import (
	"context"
	"maps"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Tracing creates OpenTelemetry spans for dispatch operations.
//
// On the dispatch side (no ReceivedStamp): starts a new span, and if the result
// is async, injects the full W3C trace context into a TraceStamp on the envelope.
//
// On the consume side (ReceivedStamp present): extracts parent trace context
// from TraceStamp.Headers and creates a child span linked to the producer.
type Tracing struct {
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// NewTracing creates a tracing middleware using the given tracer name.
func NewTracing(tracerName string) *Tracing {
	return &Tracing{
		tracer:     otel.Tracer(tracerName),
		propagator: otel.GetTextMapPropagator(),
	}
}

// NewTracingWithTracer creates a tracing middleware with an explicit tracer.
func NewTracingWithTracer(tracer trace.Tracer) *Tracing {
	return &Tracing{
		tracer:     tracer,
		propagator: otel.GetTextMapPropagator(),
	}
}

// NewTracingFull creates a tracing middleware with explicit tracer and propagator.
func NewTracingFull(tracer trace.Tracer, propagator propagation.TextMapPropagator) *Tracing {
	return &Tracing{tracer: tracer, propagator: propagator}
}

func (m *Tracing) Handle(ctx context.Context, env messenger.Envelope, next messenger.MiddlewareNext) (messenger.DispatchResult, error) {
	msgType := env.MessageTypeName()
	isConsumed := env.HasStamp(stamp.NameReceived)

	if isConsumed {
		ctx = m.extractParentContext(ctx, env)
	}

	prefix := "messenger.dispatch "
	if isConsumed {
		prefix = "messenger.consume "
	}

	ctx, span := m.tracer.Start(ctx, prefix+msgType,
		trace.WithAttributes(
			attribute.String("message.type", msgType),
			attribute.Int("message.stamps", env.StampCount()),
			attribute.Bool("message.consumed", isConsumed),
		),
	)
	defer span.End()

	result, err := next(ctx, env)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return result, err
	}

	if result.Async {
		span.SetAttributes(attribute.Bool("message.async", true))
		result.Envelope = m.injectTraceContext(ctx, result.Envelope, span)
	}

	return result, nil
}

// extractParentContext extracts parent trace from TraceStamp.Headers.
// Falls back to no-op if TraceStamp is absent or has no Headers.
func (m *Tracing) extractParentContext(ctx context.Context, env messenger.Envelope) context.Context {
	raw := env.Last(stamp.NameTrace)
	if raw == nil {
		return ctx
	}
	ts, ok := raw.(stamp.TraceStamp)
	if !ok || len(ts.Headers) == 0 {
		return ctx
	}
	// Read-only: Extract only calls Get/Keys, never Set — safe to share map.
	return m.propagator.Extract(ctx, propagation.MapCarrier(ts.Headers))
}

// injectTraceContext injects the current span context into a TraceStamp with W3C headers.
// Also preserves TraceID/SpanID for backward compatibility with old consumers.
func (m *Tracing) injectTraceContext(ctx context.Context, env messenger.Envelope, span trace.Span) messenger.Envelope {
	carrier := propagation.MapCarrier{}
	m.propagator.Inject(ctx, carrier)

	spanCtx := span.SpanContext()
	return env.WithStamp(stamp.TraceStamp{
		TraceID: spanCtx.TraceID().String(),
		SpanID:  spanCtx.SpanID().String(),
		Headers: maps.Clone(map[string]string(carrier)),
	})
}
