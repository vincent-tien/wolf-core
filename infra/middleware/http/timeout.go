// timeout.go — Per-request deadline middleware that cancels context after configured duration.
package http

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout returns a Gin middleware that wraps each request's context with a
// deadline. If the handler chain does not complete within duration, the context
// is cancelled, signalling downstream operations (DB queries, HTTP clients) to
// abort. The middleware does not write a response on timeout — that is the
// responsibility of the handler or a recovery middleware.
func Timeout(duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), duration)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
