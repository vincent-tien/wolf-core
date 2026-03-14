package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	sharedauth "github.com/vincent-tien/wolf-core/auth"
	sharederrors "github.com/vincent-tien/wolf-core/errors"
)

// mockTokenValidator is a test double for sharedauth.TokenValidator.
type mockTokenValidator struct {
	claims *sharedauth.UserClaims
	err    error
}

func (m *mockTokenValidator) ValidateAccessToken(_ context.Context, _ string) (*sharedauth.UserClaims, error) {
	return m.claims, m.err
}

// testClaims builds a sample UserClaims for use in tests.
func testClaims() *sharedauth.UserClaims {
	return &sharedauth.UserClaims{
		UserID:    "user-123",
		Email:     "alice@example.com",
		Roles:     []string{"editor"},
		SessionID: "sess-abc",
		IssuedAt:  time.Now().Add(-time.Minute),
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

// setupAuthRouter constructs a test Gin engine with AuthMiddleware applied and
// a single GET /protected route that returns 200 on success.
func setupAuthRouter(validator sharedauth.TokenValidator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	mw := NewAuthMiddleware(validator, zap.NewNop())
	r.Use(mw.Handler())

	r.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	// Arrange
	claims := testClaims()
	v := &mockTokenValidator{claims: claims}
	r := setupAuthRouter(v)

	// Act
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_ValidToken_ClaimsInContext(t *testing.T) {
	// Arrange
	claims := testClaims()
	v := &mockTokenValidator{claims: claims}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	mw := NewAuthMiddleware(v, zap.NewNop())
	r.Use(mw.Handler())

	var capturedClaims *sharedauth.UserClaims
	var capturedUserID string
	r.GET("/protected", func(c *gin.Context) {
		capturedClaims = sharedauth.ClaimsFromContext(c.Request.Context())
		uid, _ := c.Get("user_id")
		capturedUserID, _ = uid.(string)
		c.Status(http.StatusOK)
	})

	// Act
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)

	// Assert
	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, capturedClaims)
	assert.Equal(t, claims.UserID, capturedClaims.UserID)
	assert.Equal(t, claims.UserID, capturedUserID)
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	// Arrange
	v := &mockTokenValidator{claims: testClaims()}
	r := setupAuthRouter(v)

	// Act
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assertErrorBody(t, w)
}

func TestAuthMiddleware_MalformedHeader(t *testing.T) {
	// Arrange — wrong scheme
	v := &mockTokenValidator{claims: testClaims()}
	r := setupAuthRouter(v)

	// Act
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Basic xyz")
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assertErrorBody(t, w)
}

func TestAuthMiddleware_EmptyToken(t *testing.T) {
	// Arrange — "Bearer " with trailing space but no token
	v := &mockTokenValidator{claims: testClaims()}
	r := setupAuthRouter(v)

	// Act
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer ")
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assertErrorBody(t, w)
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	// Arrange
	v := &mockTokenValidator{err: sharederrors.NewTokenExpired()}
	r := setupAuthRouter(v)

	// Act
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assertErrorBody(t, w)
}

func TestAuthMiddleware_InvalidSignature(t *testing.T) {
	// Arrange
	v := &mockTokenValidator{err: sharederrors.NewTokenInvalid("signature verification failed")}
	r := setupAuthRouter(v)

	// Act
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer tampered-token")
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assertErrorBody(t, w)
}

// assertErrorBody verifies the response carries a JSON object with an "error"
// field so callers get a machine-readable message.
func assertErrorBody(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]string
	err := json.NewDecoder(w.Body).Decode(&body)
	require.NoError(t, err, "response body must be valid JSON")
	assert.NotEmpty(t, body["error"], "response must contain an 'error' field")
}
