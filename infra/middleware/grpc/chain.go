// chain.go — Ordered interceptor chain builder for the gRPC server.
package grpc

import (
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/vincent-tien/wolf-core/infra/observability/metrics"
)

// BuildInterceptors returns the ordered default unary server interceptor chain.
//
// Order:
//  1. Recovery      – catch panics and return codes.Internal.
//  2. Tracing       – instrument with OpenTelemetry distributed tracing.
//  3. Logging       – emit a structured log entry for each call.
//  4. Metrics       – record request count and duration.
//  5. ErrorMapping  – translate AppError values to gRPC status codes.
//
// Auth, RBAC, and Transaction interceptors remain per-service (not global).
//
// The chain is intended to be passed to grpc.ChainUnaryInterceptor when
// constructing the gRPC server:
//
//	grpc.NewServer(grpc.ChainUnaryInterceptor(BuildInterceptors(logger, m)...))
func BuildInterceptors(logger *zap.Logger, m *metrics.Metrics) []grpc.UnaryServerInterceptor {
	return []grpc.UnaryServerInterceptor{
		Recovery(logger),
		TracingUnaryInterceptor(),
		Logging(logger),
		MetricsUnaryInterceptor(m),
		ErrorMappingInterceptor(),
	}
}
