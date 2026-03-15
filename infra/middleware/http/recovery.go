// recovery.go — Panic recovery middleware that logs stack trace and returns 500.
package http

import (
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	wolfhttp "github.com/vincent-tien/wolf-core/infra/http"
)

// Recovery returns a Gin middleware that catches any panic propagating through
// the handler chain, logs the panic value and a full stack trace at error
// level, and writes a generic 500 JSON response to the client. Stack traces
// are never forwarded to the client to prevent internal detail leakage.
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				logger.Error("http: panic recovered",
					zap.Any("panic", r),
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.ByteString("stack", stack),
				)
				wolfhttp.AbortInternalError(c, "an unexpected error occurred")
			}
		}()
		c.Next()
	}
}
