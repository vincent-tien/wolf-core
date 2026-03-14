// Package grpc provides gRPC server interceptors for the wolf-be service.
// Each interceptor is a standalone constructor so they can be composed via
// BuildInterceptors or registered individually using grpc.ChainUnaryInterceptor.
package grpc

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/vincent-tien/wolf-core/infra/observability/logging"
	sharedauth "github.com/vincent-tien/wolf-core/auth"
)

const metadataRequestIDKey = "x-request-id"

// Logging returns a gRPC unary server interceptor that emits a structured log
// entry for every RPC call. The log record includes:
//   - method     – fully-qualified gRPC method name
//   - duration   – wall-clock duration of the handler
//   - code       – gRPC status code string (e.g. "OK", "NotFound")
//   - trace_id   – OpenTelemetry trace ID for log-to-trace correlation
//   - request_id – from gRPC metadata (if present)
//   - user_id    – authenticated user ID (if present)
//
// Calls that return an error are logged at error level; successful calls are
// logged at info level.
func Logging(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)

		st, _ := status.FromError(err)
		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", time.Since(start)),
			zap.String("code", st.Code().String()),
		}

		if tid := logging.OTelTraceIDFromContext(ctx); tid != "" {
			fields = append(fields, zap.String("trace_id", tid))
		}

		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(metadataRequestIDKey); len(vals) > 0 {
				fields = append(fields, zap.String("request_id", vals[0]))
			}
		}

		if claims := sharedauth.ClaimsFromContext(ctx); claims != nil {
			fields = append(fields, zap.String("user_id", claims.UserID))
		}

		if err != nil {
			logger.Error("grpc request", append(fields, zap.Error(err))...)
		} else {
			logger.Info("grpc request", fields...)
		}

		return resp, err
	}
}
