package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	httpmw "github.com/vincent-tien/wolf-core/infra/middleware/http"
)

// newAPIKeyRouter builds a test Gin engine that applies APIKeyMiddleware and
// exposes GET /test returning 200 on success.
func newAPIKeyRouter(keys map[string]string) *gin.Engine {
	r := gin.New()
	mw := httpmw.NewAPIKeyMiddleware(keys)
	r.Use(mw.Handler())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestAPIKey_ValidKey(t *testing.T) {
	t.Parallel()

	// Arrange
	keys := map[string]string{"order-service": "super-secret-key"}
	r := newAPIKeyRouter(keys)

	// Act
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "super-secret-key")
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKey_InvalidKey(t *testing.T) {
	t.Parallel()

	// Arrange
	keys := map[string]string{"order-service": "super-secret-key"}
	r := newAPIKeyRouter(keys)

	// Act
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKey_MissingKey(t *testing.T) {
	t.Parallel()

	// Arrange
	keys := map[string]string{"order-service": "super-secret-key"}
	r := newAPIKeyRouter(keys)

	// Act — no X-API-Key header set.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKey_EmptyKeys(t *testing.T) {
	t.Parallel()

	// Arrange — middleware configured with no registered keys.
	r := newAPIKeyRouter(map[string]string{})

	// Act — even a "valid-looking" key must be rejected.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-API-Key", "any-key")
	r.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
