// auth.go — gRPC unary/stream interceptor for JWT Bearer token authentication.
package grpc

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	sharedauth "github.com/vincent-tien/wolf-core/auth"
	sharederrors "github.com/vincent-tien/wolf-core/errors"
)

// AuthInterceptor validates JWT tokens carried in the gRPC "authorization"
// metadata header. Public methods listed at construction time bypass token
// validation entirely.
type AuthInterceptor struct {
	validator     sharedauth.TokenValidator
	logger        *zap.Logger
	publicMethods map[string]struct{}
}

// NewAuthInterceptor constructs an AuthInterceptor. publicMethods is a slice
// of fully-qualified gRPC method names (e.g. "/order.v1.OrderService/GetOrder")
// that do not require authentication.
func NewAuthInterceptor(
	validator sharedauth.TokenValidator,
	logger *zap.Logger,
	publicMethods []string,
) *AuthInterceptor {
	pm := make(map[string]struct{}, len(publicMethods))
	for _, m := range publicMethods {
		pm[m] = struct{}{}
	}
	return &AuthInterceptor{
		validator:     validator,
		logger:        logger,
		publicMethods: pm,
	}
}

// UnaryInterceptor returns a grpc.UnaryServerInterceptor that:
//  1. Skips auth for public methods.
//  2. Extracts "authorization: Bearer <token>" from gRPC metadata.
//  3. Validates the token and injects UserClaims into the context.
//  4. Maps auth failures to codes.Unauthenticated and forbidden failures to
//     codes.PermissionDenied before returning.
func (i *AuthInterceptor) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if _, ok := i.publicMethods[info.FullMethod]; ok {
			return handler(ctx, req)
		}

		tokenString, err := extractBearerToken(ctx)
		if err != nil {
			return nil, err
		}

		claims, err := i.validator.ValidateAccessToken(ctx, tokenString)
		if err != nil {
			i.logger.Debug("grpc token validation failed",
				zap.String("method", info.FullMethod),
				zap.Error(err),
			)
			return nil, mapAuthError(err)
		}

		ctx = sharedauth.WithClaims(ctx, claims)
		return handler(ctx, req)
	}
}

// extractBearerToken reads the "authorization" key from gRPC incoming metadata
// and parses the "Bearer <token>" value. Returns a gRPC status error when the
// header is absent or malformed.
func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", status.Error(codes.Unauthenticated, "authorization header is required")
	}

	raw := values[0]
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) {
		return "", status.Error(codes.Unauthenticated, "authorization header must be 'Bearer <token>'")
	}

	token := strings.TrimSpace(raw[len(prefix):])
	if token == "" {
		return "", status.Error(codes.Unauthenticated, "authorization token is empty")
	}

	return token, nil
}

// mapAuthError converts a token validation error into the appropriate gRPC
// status error. Forbidden errors become codes.PermissionDenied; everything
// else becomes codes.Unauthenticated.
func mapAuthError(err error) error {
	var appErr *sharederrors.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case sharederrors.ErrForbidden:
			return status.Error(codes.PermissionDenied, appErr.Message)
		default:
			return status.Error(codes.Unauthenticated, appErr.Message)
		}
	}

	return status.Error(codes.Unauthenticated, err.Error())
}
