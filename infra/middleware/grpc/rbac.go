// rbac.go — Role-Based Access Control interceptor checking permission claims.
package grpc

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sharedauth "github.com/vincent-tien/wolf-core/auth"
	sharederrors "github.com/vincent-tien/wolf-core/errors"
)

// RBACRule declares the authorization requirements for a single gRPC method.
// At least one role must match (OR semantics) AND all listed permissions must
// be present (AND semantics). An empty Roles slice skips the role check; an
// empty Permissions slice skips the permission check.
type RBACRule struct {
	// Roles lists acceptable roles — access is granted if the user holds any one.
	Roles []string
	// Permissions lists required permissions — the user must hold ALL of them.
	Permissions []string
}

// RBACInterceptor enforces per-method role and permission rules on gRPC calls.
// Methods not present in the rules map are allowed through without any check.
type RBACInterceptor struct {
	rules  map[string]RBACRule
	logger *zap.Logger
}

// NewRBACInterceptor constructs an RBACInterceptor. The rules map keys are
// fully-qualified gRPC method names (e.g. "/order.v1.OrderService/CreateOrder").
func NewRBACInterceptor(rules map[string]RBACRule, logger *zap.Logger) *RBACInterceptor {
	return &RBACInterceptor{
		rules:  rules,
		logger: logger,
	}
}

// UnaryInterceptor returns a grpc.UnaryServerInterceptor that enforces RBAC
// rules. It must run after AuthInterceptor so that UserClaims are already
// present in the context. Methods without a matching rule are passed through
// unchanged.
func (i *RBACInterceptor) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		rule, ok := i.rules[info.FullMethod]
		if !ok {
			return handler(ctx, req)
		}

		claims := sharedauth.ClaimsFromContext(ctx)
		if claims == nil {
			return nil, status.Error(codes.Unauthenticated, sharederrors.NewUnauthorized("authentication required").Error())
		}

		if len(rule.Roles) > 0 && !claims.HasAnyRole(rule.Roles...) {
			i.logger.Debug("grpc insufficient role",
				zap.String("method", info.FullMethod),
				zap.Strings("required", rule.Roles),
				zap.Strings("actual", claims.Roles),
				zap.String("user_id", claims.UserID),
			)
			return nil, status.Error(codes.PermissionDenied, sharederrors.NewInsufficientRole(rule.Roles).Error())
		}

		for _, perm := range rule.Permissions {
			if !claims.HasPermission(perm) {
				i.logger.Debug("grpc insufficient permission",
					zap.String("method", info.FullMethod),
					zap.String("missing", perm),
					zap.String("user_id", claims.UserID),
				)
				return nil, status.Error(codes.PermissionDenied, sharederrors.NewInsufficientPermission(perm).Error())
			}
		}

		return handler(ctx, req)
	}
}
