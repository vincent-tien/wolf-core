package middleware_test

import (
	"context"
	"testing"
	"time"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/middleware"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func newTestTracing(t *testing.T) (*middleware.Tracing, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	tracer := tp.Tracer("test")
	prop := propagation.TraceContext{}
	return middleware.NewTracingFull(tracer, prop), exporter
}

func TestTracing_DispatchSync_CreatesSpan(t *testing.T) {
	mw, exporter := newTestTracing(t)
	env := messenger.NewEnvelope(testCmd{ID: "1"})

	_, err := mw.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if got := spans[0].Name; got != "messenger.dispatch test.Cmd" {
		t.Errorf("span name = %q", got)
	}
}

func TestTracing_DispatchAsync_InjectsTraceStamp(t *testing.T) {
	mw, exporter := newTestTracing(t)
	env := messenger.NewEnvelope(testCmd{ID: "2"})

	asyncNext := func(_ context.Context, env messenger.Envelope) (messenger.DispatchResult, error) {
		return messenger.DispatchResult{Envelope: env, Async: true}, nil
	}

	result, err := mw.Handle(context.Background(), env, asyncNext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have TraceStamp with Headers.
	raw := result.Envelope.Last(stamp.NameTrace)
	if raw == nil {
		t.Fatal("expected TraceStamp on async result")
	}
	ts := raw.(stamp.TraceStamp)
	if ts.TraceID == "" {
		t.Error("TraceStamp.TraceID should be set")
	}
	if ts.SpanID == "" {
		t.Error("TraceStamp.SpanID should be set")
	}
	if len(ts.Headers) == 0 {
		t.Error("TraceStamp.Headers should contain W3C propagation headers")
	}
	if _, ok := ts.Headers["traceparent"]; !ok {
		t.Error("TraceStamp.Headers should contain 'traceparent'")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
}

func TestTracing_ConsumesSide_ExtractsParentTrace(t *testing.T) {
	mw, exporter := newTestTracing(t)

	// Simulate a producer envelope with TraceStamp containing W3C headers.
	producerHeaders := map[string]string{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}
	env := messenger.NewEnvelope(testCmd{ID: "3"},
		stamp.ReceivedStamp{Transport: "memory", ReceivedAt: time.Now()},
		stamp.TraceStamp{
			TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
			SpanID:  "00f067aa0ba902b7",
			Headers: producerHeaders,
		},
	)

	_, err := mw.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}

	span := spans[0]
	if got := span.Name; got != "messenger.consume test.Cmd" {
		t.Errorf("span name = %q, want consume prefix", got)
	}

	// Parent trace should match the injected traceparent.
	if span.Parent.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("parent TraceID = %q, want producer trace", span.Parent.TraceID().String())
	}
}

func TestTracing_ConsumesSide_MissingTraceStamp_NoError(t *testing.T) {
	mw, _ := newTestTracing(t)

	env := messenger.NewEnvelope(testCmd{ID: "4"},
		stamp.ReceivedStamp{Transport: "memory", ReceivedAt: time.Now()},
	)

	_, err := mw.Handle(context.Background(), env, passThrough)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTracing_ErrorRecordedOnSpan(t *testing.T) {
	mw, exporter := newTestTracing(t)
	env := messenger.NewEnvelope(testCmd{ID: "5"})

	_, err := mw.Handle(context.Background(), env, failNext)
	if err == nil {
		t.Fatal("expected error")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if len(spans[0].Events) == 0 {
		t.Error("span should have error event recorded")
	}
}

func TestTraceStamp_MapCarrier_Roundtrip(t *testing.T) {
	carrier := propagation.MapCarrier{}
	carrier.Set("traceparent", "00-abc-def-01")
	carrier.Set("tracestate", "vendor=opaque")

	ts := stamp.TraceStamp{Headers: map[string]string(carrier)}
	if ts.Headers["traceparent"] != "00-abc-def-01" {
		t.Errorf("traceparent = %q", ts.Headers["traceparent"])
	}

	// Roundtrip: stamp → carrier → verify.
	carrier2 := propagation.MapCarrier(ts.Headers)
	if got := carrier2.Get("traceparent"); got != "00-abc-def-01" {
		t.Errorf("roundtrip traceparent = %q", got)
	}
	if got := carrier2.Get("tracestate"); got != "vendor=opaque" {
		t.Errorf("roundtrip tracestate = %q", got)
	}

	keys := carrier2.Keys()
	if len(keys) != 2 {
		t.Errorf("Keys() len = %d, want 2", len(keys))
	}
}

func TestTraceStamp_EmptyHeaders(t *testing.T) {
	carrier := propagation.MapCarrier(map[string]string{})
	if got := carrier.Get("missing"); got != "" {
		t.Errorf("Get on empty carrier = %q, want empty", got)
	}
}

func TestTraceStamp_BackwardCompat_OldFormat(t *testing.T) {
	ts := stamp.TraceStamp{TraceID: "abc", SpanID: "def"}
	if ts.StampName() != stamp.NameTrace {
		t.Errorf("StampName() = %q, want %q", ts.StampName(), stamp.NameTrace)
	}
	if len(ts.Headers) != 0 {
		t.Error("old format stamp should have nil headers")
	}
}
