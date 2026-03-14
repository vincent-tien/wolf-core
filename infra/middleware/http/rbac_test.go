package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	sharedauth "github.com/vincent-tien/wolf-core/auth"
)

// injectClaims returns a gin.HandlerFunc that stores the given claims in the
// request context, simulating what AuthMiddleware does in production.
func injectClaims(claims *sharedauth.UserClaims) gin.HandlerFunc {
	return func(c *gin.Context) {
		if claims != nil {
			ctx := sharedauth.WithClaims(c.Request.Context(), claims)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}

// okHandler is a simple Gin handler that returns 200 OK.
func okHandler(c *gin.Context) { c.Status(http.StatusOK) }

// newRBACRouter builds a test Gin engine that injects claims and then applies
// the given RBAC middleware before the okHandler.
func newRBACRouter(claims *sharedauth.UserClaims, rbacMW gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/resource/:id", injectClaims(claims), rbacMW, okHandler)
	return r
}

// newRBACRouterNoParam builds a test Gin engine without a path parameter.
func newRBACRouterNoParam(claims *sharedauth.UserClaims, rbacMW gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/resource", injectClaims(claims), rbacMW, okHandler)
	return r
}

func TestRequireRoles_HasRole(t *testing.T) {
	// Arrange — user has "admin", middleware requires "admin"
	claims := &sharedauth.UserClaims{UserID: "u1", Roles: []string{"admin"}}
	rbac := NewRBACMiddleware(zap.NewNop())
	r := newRBACRouterNoParam(claims, rbac.RequireRoles("admin"))

	// Act
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/resource", nil))

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRoles_MissingRole(t *testing.T) {
	// Arrange — user has "editor", middleware requires "admin"
	claims := &sharedauth.UserClaims{UserID: "u1", Roles: []string{"editor"}}
	rbac := NewRBACMiddleware(zap.NewNop())
	r := newRBACRouterNoParam(claims, rbac.RequireRoles("admin"))

	// Act
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/resource", nil))

	// Assert
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireRoles_NoClaims(t *testing.T) {
	// Arrange — no claims in context (auth middleware not applied)
	rbac := NewRBACMiddleware(zap.NewNop())
	r := newRBACRouterNoParam(nil, rbac.RequireRoles("admin"))

	// Act
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/resource", nil))

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequirePermissions_HasAll(t *testing.T) {
	// Arrange — user has all required permissions
	claims := &sharedauth.UserClaims{
		UserID:      "u1",
		Permissions: []string{"orders:read", "orders:write"},
	}
	rbac := NewRBACMiddleware(zap.NewNop())
	r := newRBACRouterNoParam(claims, rbac.RequirePermissions("orders:read", "orders:write"))

	// Act
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/resource", nil))

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequirePermissions_MissingOne(t *testing.T) {
	// Arrange — user has only one of the required permissions
	claims := &sharedauth.UserClaims{
		UserID:      "u1",
		Permissions: []string{"orders:read"},
	}
	rbac := NewRBACMiddleware(zap.NewNop())
	r := newRBACRouterNoParam(claims, rbac.RequirePermissions("orders:read", "orders:write"))

	// Act
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/resource", nil))

	// Assert
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireSelf_OwnResource(t *testing.T) {
	// Arrange — claims.UserID matches the :id param
	claims := &sharedauth.UserClaims{UserID: "u1", Roles: []string{"editor"}}
	rbac := NewRBACMiddleware(zap.NewNop())
	r := newRBACRouter(claims, rbac.RequireSelf("id"))

	// Act
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/resource/u1", nil))

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireSelf_OtherResource_Denied(t *testing.T) {
	// Arrange — claims.UserID does NOT match the :id param and no admin role
	claims := &sharedauth.UserClaims{UserID: "u1", Roles: []string{"editor"}}
	rbac := NewRBACMiddleware(zap.NewNop())
	r := newRBACRouter(claims, rbac.RequireSelf("id"))

	// Act
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/resource/u2", nil))

	// Assert
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRequireSelf_Admin_Bypass(t *testing.T) {
	// Arrange — user has "admin" role so can access any resource
	claims := &sharedauth.UserClaims{UserID: "admin-user", Roles: []string{"admin"}}
	rbac := NewRBACMiddleware(zap.NewNop())
	r := newRBACRouter(claims, rbac.RequireSelf("id"))

	// Act — request targets a different user's resource
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/resource/some-other-user", nil))

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
}
