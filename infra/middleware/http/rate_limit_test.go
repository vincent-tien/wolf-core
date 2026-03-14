package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	httpmw "github.com/vincent-tien/wolf-core/infra/middleware/http"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestPerIPRateLimiter_AllowsUnderLimit(t *testing.T) {
	t.Parallel()

	limiter := httpmw.NewPerIPRateLimiter(10, 10)
	router := gin.New()
	router.Use(limiter.Middleware())
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPerIPRateLimiter_RejectsOverLimit(t *testing.T) {
	t.Parallel()

	// 1 RPS, burst 1 — second request should be rejected.
	limiter := httpmw.NewPerIPRateLimiter(1, 1)
	router := gin.New()
	router.Use(limiter.Middleware())
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// First request — uses the burst token.
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "10.0.0.1:5678"
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Second request — no tokens left.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.1:5678"
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)
}

func TestPerIPRateLimiter_DifferentIPsIndependent(t *testing.T) {
	t.Parallel()

	limiter := httpmw.NewPerIPRateLimiter(1, 1)
	router := gin.New()
	router.Use(limiter.Middleware())
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// IP A exhausts its tokens.
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "10.0.0.1:1111"
	router.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// IP B still has tokens.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.2:2222"
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestPerIPRateLimiter_RetryAfterHeader(t *testing.T) {
	t.Parallel()

	limiter := httpmw.NewPerIPRateLimiter(1, 1)
	router := gin.New()
	router.Use(limiter.Middleware())
	router.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Exhaust tokens.
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "10.0.0.3:3333"
	router.ServeHTTP(w1, req1)

	// Second request — should have Retry-After header.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.3:3333"
	router.ServeHTTP(w2, req2)
	assert.Equal(t, "1", w2.Header().Get("Retry-After"))
}
