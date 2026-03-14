package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/vincent-tien/wolf-core/infra/config"
	httpmw "github.com/vincent-tien/wolf-core/infra/middleware/http"
)

func defaultSecurityHeadersCfg() config.SecurityHeadersConfig {
	return config.SecurityHeadersConfig{
		Enabled:            true,
		HSTS:               true,
		HSTSMaxAge:         31536000,
		ContentTypeNosniff: true,
		FrameDeny:          true,
		XSSProtection:      true,
		ReferrerPolicy:     "strict-origin-when-cross-origin",
	}
}

// newSecurityHeadersRouter builds a test Gin engine that applies SecurityHeaders
// middleware and exposes GET /test returning 200.
func newSecurityHeadersRouter(cfg config.SecurityHeadersConfig) *gin.Engine {
	r := gin.New()
	r.Use(httpmw.SecurityHeaders(cfg))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestSecurityHeaders_AllHeadersPresent(t *testing.T) {
	t.Parallel()

	cfg := defaultSecurityHeadersCfg()
	cfg.ContentSecurityPolicy = "default-src 'self'"
	r := newSecurityHeadersRouter(cfg)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("Strict-Transport-Security"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "1; mode=block", w.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "default-src 'self'", w.Header().Get("Content-Security-Policy"))
	assert.NotEmpty(t, w.Header().Get("Referrer-Policy"))
}

func TestSecurityHeaders_Disabled(t *testing.T) {
	t.Parallel()

	cfg := config.SecurityHeadersConfig{Enabled: false}
	r := newSecurityHeadersRouter(cfg)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Strict-Transport-Security"))
	assert.Empty(t, w.Header().Get("X-Content-Type-Options"))
	assert.Empty(t, w.Header().Get("X-Frame-Options"))
	assert.Empty(t, w.Header().Get("X-XSS-Protection"))
	assert.Empty(t, w.Header().Get("Content-Security-Policy"))
}

func TestSecurityHeaders_CustomCSP(t *testing.T) {
	t.Parallel()

	customCSP := "default-src 'none'; script-src 'self'; connect-src 'self'"
	cfg := defaultSecurityHeadersCfg()
	cfg.ContentSecurityPolicy = customCSP
	r := newSecurityHeadersRouter(cfg)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, customCSP, w.Header().Get("Content-Security-Policy"))
}

func TestSecurityHeaders_HSTSDisabled(t *testing.T) {
	t.Parallel()

	cfg := defaultSecurityHeadersCfg()
	cfg.HSTS = false
	r := newSecurityHeadersRouter(cfg)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Strict-Transport-Security"),
		"HSTS header must be absent when HSTS=false")
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
}
