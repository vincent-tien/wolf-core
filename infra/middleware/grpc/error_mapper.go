// error_mapper.go — Maps AppError codes to gRPC status codes.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/vincent-tien/wolf-core/errors"
)

// ErrorMappingInterceptor returns a gRPC unary server interceptor that
// translates apperrors.AppError values returned by handlers into the
// appropriate gRPC status codes. Errors that are not AppError instances are
// wrapped as codes.Internal.
//
// Mapping:
//   - ErrNotFound     → codes.NotFound
//   - ErrValidation   → codes.InvalidArgument
//   - ErrConflict     → codes.AlreadyExists
//   - ErrUnauthorized → codes.Unauthenticated
//   - ErrForbidden    → codes.PermissionDenied
//   - ErrInternal     → codes.Internal
//   - ErrRateLimited  → codes.ResourceExhausted
//   - unknown         → codes.Internal
func ErrorMappingInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		// If err is already a gRPC status error, pass it through unchanged so
		// that handlers which explicitly set status codes are not overridden.
		if _, ok := status.FromError(err); ok {
			return nil, err
		}

		return nil, mapGRPCError(err)
	}
}

// mapGRPCError converts err into a gRPC status error. If err wraps an
// AppError the conversion is deterministic; otherwise codes.Internal is used.
func mapGRPCError(err error) error {
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		return status.Errorf(codes.Internal, "internal server error")
	}

	switch appErr.Code {
	case apperrors.ErrNotFound:
		return status.Error(codes.NotFound, appErr.Message)
	case apperrors.ErrValidation:
		return status.Error(codes.InvalidArgument, appErr.Message)
	case apperrors.ErrConflict:
		return status.Error(codes.AlreadyExists, appErr.Message)
	case apperrors.ErrUnauthorized:
		return status.Error(codes.Unauthenticated, appErr.Message)
	case apperrors.ErrForbidden:
		return status.Error(codes.PermissionDenied, appErr.Message)
	case apperrors.ErrInternal:
		return status.Error(codes.Internal, appErr.Message)
	case apperrors.ErrRateLimited:
		return status.Error(codes.ResourceExhausted, appErr.Message)
	default:
		return status.Error(codes.Internal, appErr.Message)
	}
}
