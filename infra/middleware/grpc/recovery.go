// recovery.go — Panic recovery interceptor that logs stack trace and returns Internal.
package grpc

import (
	"context"
	"runtime/debug"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Recovery returns a gRPC unary server interceptor that catches any panic
// propagating out of the handler, logs the panic value and a full stack trace
// at error level, and returns a generic codes.Internal status to the client.
// Stack traces are never forwarded to the caller to prevent internal detail leakage.
func Recovery(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				logger.Error("grpc: panic recovered",
					zap.Any("panic", r),
					zap.String("method", info.FullMethod),
					zap.ByteString("stack", stack),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}
