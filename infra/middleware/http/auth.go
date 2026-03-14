// auth.go — JWT Bearer token authentication middleware for Gin.
package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	sharedauth "github.com/vincent-tien/wolf-core/auth"
	sharederrors "github.com/vincent-tien/wolf-core/errors"
)

// AuthMiddleware validates JWT tokens extracted from the Authorization header.
// It injects verified UserClaims into the request context for downstream handlers.
type AuthMiddleware struct {
	validator sharedauth.TokenValidator
	logger    *zap.Logger
}

// NewAuthMiddleware constructs an AuthMiddleware with the given token validator and logger.
func NewAuthMiddleware(validator sharedauth.TokenValidator, logger *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		validator: validator,
		logger:    logger,
	}
}

// Handler returns a gin.HandlerFunc that enforces JWT authentication on every request.
//
// It expects an "Authorization: Bearer <token>" header. Missing or malformed
// headers produce a 401 response. Token validation errors are mapped to the
// appropriate status code via writeAuthError before aborting the chain.
func (m *AuthMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if raw == "" {
			writeAuthError(c, sharederrors.NewUnauthorized("authorization header is required"))
			return
		}

		tokenString, ok := parseBearerToken(raw)
		if !ok {
			writeAuthError(c, sharederrors.NewUnauthorized("authorization header must be 'Bearer <token>'"))
			return
		}

		claims, err := m.validator.ValidateAccessToken(c.Request.Context(), tokenString)
		if err != nil {
			m.logger.Debug("token validation failed", zap.Error(err))
			writeAuthError(c, err)
			return
		}

		ctx := sharedauth.WithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Set("user_id", claims.UserID)

		c.Next()
	}
}

// parseBearerToken extracts the token string from a "Bearer <token>" header
// value. Returns the token and true on success, or empty string and false when
// the format is missing, incorrect scheme, or the token portion is empty.
func parseBearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// writeAuthError maps err to an HTTP status code and writes a JSON error
// response before aborting the Gin handler chain.
//
// Mapping rules:
//   - sharederrors.ErrForbidden → 403
//   - sharederrors.ErrUnauthorized (and all auth sub-errors) → 401
//   - unknown → 401
func writeAuthError(c *gin.Context, err error) {
	statusCode := http.StatusUnauthorized

	var appErr *sharederrors.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case sharederrors.ErrForbidden:
			statusCode = http.StatusForbidden
		default:
			statusCode = http.StatusUnauthorized
		}
	}

	c.AbortWithStatusJSON(statusCode, gin.H{"error": err.Error()})
}
