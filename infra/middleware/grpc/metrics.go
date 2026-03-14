// metrics.go — Prometheus gRPC request metrics (duration histogram, request counter).
package grpc

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/vincent-tien/wolf-core/infra/observability/metrics"
)

// MetricsUnaryInterceptor returns a gRPC unary server interceptor that records
// request count and duration into the provided Prometheus collectors:
//   - GRPCRequestTotal: counter labelled by full method name and gRPC status code.
//   - GRPCRequestDuration: histogram labelled by full method name and gRPC status code.
func MetricsUnaryInterceptor(m *metrics.Metrics) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		st, _ := status.FromError(err)
		code := st.Code().String()
		elapsed := time.Since(start).Seconds()

		m.GRPCRequestTotal.WithLabelValues(info.FullMethod, code).Inc()
		m.GRPCRequestDuration.WithLabelValues(info.FullMethod, code).Observe(elapsed)

		return resp, err
	}
}
