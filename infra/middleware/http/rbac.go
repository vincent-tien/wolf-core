// rbac.go — Role-Based Access Control middleware checking permission claims.
package http

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	sharedauth "github.com/vincent-tien/wolf-core/auth"
	sharederrors "github.com/vincent-tien/wolf-core/errors"
	wolfhttp "github.com/vincent-tien/wolf-core/infra/http"
)

// RBACMiddleware provides role-based and permission-based access control
// middleware factories. It must be used after AuthMiddleware so that
// UserClaims are already present in the context.
type RBACMiddleware struct {
	logger *zap.Logger
}

// NewRBACMiddleware constructs an RBACMiddleware with the given logger.
func NewRBACMiddleware(logger *zap.Logger) *RBACMiddleware {
	return &RBACMiddleware{logger: logger}
}

// RequireRoles returns a gin.HandlerFunc that aborts with 403 unless the
// authenticated user has at least one of the specified roles. If no claims are
// present in the context the request is aborted with 401.
func (m *RBACMiddleware) RequireRoles(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := sharedauth.ClaimsFromContext(c.Request.Context())
		if claims == nil {
			wolfhttp.AbortUnauthorized(c, sharederrors.NewUnauthorized("authentication required").Error())
			return
		}

		if !claims.HasAnyRole(roles...) {
			m.logger.Debug("insufficient role",
				zap.Strings("required", roles),
				zap.Strings("actual", claims.Roles),
				zap.String("user_id", claims.UserID),
			)
			wolfhttp.AbortForbidden(c, sharederrors.NewInsufficientRole(roles).Error())
			return
		}

		c.Next()
	}
}

// RequirePermissions returns a gin.HandlerFunc that aborts with 403 unless the
// authenticated user holds ALL of the specified permissions. The check stops
// and returns 403 on the first missing permission. If no claims are present the
// request is aborted with 401.
func (m *RBACMiddleware) RequirePermissions(perms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := sharedauth.ClaimsFromContext(c.Request.Context())
		if claims == nil {
			wolfhttp.AbortUnauthorized(c, sharederrors.NewUnauthorized("authentication required").Error())
			return
		}

		for _, perm := range perms {
			if !claims.HasPermission(perm) {
				m.logger.Debug("insufficient permission",
					zap.String("missing", perm),
					zap.String("user_id", claims.UserID),
				)
				wolfhttp.AbortForbidden(c, sharederrors.NewInsufficientPermission(perm).Error())
				return
			}
		}

		c.Next()
	}
}

// RequireSelf returns a gin.HandlerFunc that allows the request only when
// claims.UserID matches the URL path parameter identified by paramName, OR the
// user holds the "admin" role. This enforces that users can only access their
// own resources unless they are administrators.
func (m *RBACMiddleware) RequireSelf(paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := sharedauth.ClaimsFromContext(c.Request.Context())
		if claims == nil {
			wolfhttp.AbortUnauthorized(c, sharederrors.NewUnauthorized("authentication required").Error())
			return
		}

		resourceID := c.Param(paramName)
		if claims.UserID == resourceID || claims.HasRole("admin") {
			c.Next()
			return
		}

		m.logger.Debug("self-check failed",
			zap.String("param", paramName),
			zap.String("resource_id", resourceID),
			zap.String("user_id", claims.UserID),
		)
		wolfhttp.AbortForbidden(c, sharederrors.NewForbidden("access denied: you can only access your own resources").Error())
	}
}

// RequireRolesOrSelf returns a gin.HandlerFunc that allows the request when
// the user either holds at least one of the given roles OR when claims.UserID
// matches the URL path parameter identified by paramName (self-access). This
// combines role-based delegation with self-service access patterns.
func (m *RBACMiddleware) RequireRolesOrSelf(paramName string, roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := sharedauth.ClaimsFromContext(c.Request.Context())
		if claims == nil {
			wolfhttp.AbortUnauthorized(c, sharederrors.NewUnauthorized("authentication required").Error())
			return
		}

		resourceID := c.Param(paramName)
		if claims.HasAnyRole(roles...) || claims.UserID == resourceID {
			c.Next()
			return
		}

		m.logger.Debug("roles-or-self check failed",
			zap.Strings("required_roles", roles),
			zap.String("param", paramName),
			zap.String("resource_id", resourceID),
			zap.String("user_id", claims.UserID),
		)
		wolfhttp.AbortForbidden(c, sharederrors.NewInsufficientRole(roles).Error())
	}
}
