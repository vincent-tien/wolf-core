// security_headers.go — Injects security headers (CSP, X-Frame-Options, HSTS, etc.).
package http

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/vincent-tien/wolf-core/infra/config"
)

// SecurityHeaders returns a Gin middleware that sets security-related HTTP headers.
// If cfg.Enabled is false, a no-op handler is returned.
func SecurityHeaders(cfg config.SecurityHeadersConfig) gin.HandlerFunc {
	if !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	maxAge := cfg.HSTSMaxAge
	if maxAge <= 0 {
		maxAge = 31536000
	}

	referrer := cfg.ReferrerPolicy
	if referrer == "" {
		referrer = "strict-origin-when-cross-origin"
	}

	hstsValue := fmt.Sprintf("max-age=%d; includeSubDomains", maxAge)

	return func(c *gin.Context) {
		if cfg.HSTS {
			c.Header("Strict-Transport-Security", hstsValue)
		}

		if cfg.ContentTypeNosniff {
			c.Header("X-Content-Type-Options", "nosniff")
		}

		if cfg.FrameDeny {
			c.Header("X-Frame-Options", "DENY")
		}

		if cfg.XSSProtection {
			c.Header("X-XSS-Protection", "1; mode=block")
		}

		if cfg.ContentSecurityPolicy != "" {
			c.Header("Content-Security-Policy", cfg.ContentSecurityPolicy)
		}

		c.Header("Referrer-Policy", referrer)

		c.Next()
	}
}
