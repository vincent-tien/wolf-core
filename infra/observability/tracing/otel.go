// Package tracing provides OpenTelemetry tracing initialisation for the
// wolf-be service. It wires together an OTLP gRPC exporter, a batch span
// processor, and a ratio-based sampler, then sets the resulting
// TracerProvider and W3C propagator as the global OTel defaults.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// serviceVersion is the default version tag embedded in trace resources.
// Override at build time via -ldflags if required.
var serviceVersion = "dev"

// Init initialises OpenTelemetry tracing and returns a *sdktrace.TracerProvider
// whose Shutdown method must be deferred by the caller to flush pending spans
// on process exit.
//
// Parameters:
//   - serviceName: the logical name of this service (service.name attribute).
//   - env:         the deployment environment (deployment.environment attribute).
//   - endpoint:    the OTLP collector gRPC endpoint, e.g. "localhost:4317".
//   - sampleRate:  fraction of traces to sample in the range [0.0, 1.0].
//
// The function also sets the global TracerProvider and a W3C TraceContext +
// Baggage composite propagator so that instrumented libraries pick them up
// automatically.
func Init(ctx context.Context, serviceName, env, endpoint string, sampleRate float64, insecure bool) (*sdktrace.TracerProvider, error) {
	exporter, err := buildExporter(ctx, endpoint, insecure)
	if err != nil {
		return nil, fmt.Errorf("tracing: create OTLP exporter: %w", err)
	}

	res, err := buildResource(ctx, serviceName, env)
	if err != nil {
		return nil, fmt.Errorf("tracing: create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(sampleRate)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return tp, nil
}

// buildExporter creates an OTLP gRPC span exporter that targets endpoint.
// When insecure is true, TLS is disabled (development only). Production
// deployments default to TLS for trace data confidentiality.
func buildExporter(ctx context.Context, endpoint string, insecure bool) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	return otlptracegrpc.New(ctx, opts...)
}

// buildResource constructs an OTel Resource with service.name,
// service.version, and deployment.environment attributes.
func buildResource(ctx context.Context, serviceName, env string) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
			attribute.String("deployment.environment", env),
		),
	)
}
