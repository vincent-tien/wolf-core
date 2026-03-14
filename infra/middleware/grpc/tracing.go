// tracing.go — OpenTelemetry distributed tracing interceptor for gRPC.
package grpc

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcstatus "google.golang.org/grpc/status"
)

const tracerName = "github.com/vincent-tien/wolf-be/middleware/grpc"

// TracingUnaryInterceptor returns a gRPC unary server interceptor that creates
// an OpenTelemetry span for each call. The span name is set to the full gRPC
// method (e.g. "/package.Service/Method"). On error, the span records the error
// and sets its status to Error. Trace context propagation through gRPC metadata
// is handled by the global OTel propagator (set during tracing init).
func TracingUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		tracer := otel.Tracer(tracerName)
		ctx, span := tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		resp, err := handler(ctx, req)
		if err != nil {
			st, _ := grpcstatus.FromError(err)
			span.RecordError(err)
			span.SetStatus(codes.Error, st.Message())
		}
		return resp, err
	}
}
