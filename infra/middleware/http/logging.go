// logging.go — Structured request/response logging middleware with duration and status.
package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/observability/logging"
	sharedauth "github.com/vincent-tien/wolf-core/auth"
)

// Logging returns a Gin middleware that emits a structured log entry for every
// HTTP request after it completes. The log record includes:
//   - method     – HTTP method (GET, POST, …)
//   - path       – request path (without query string)
//   - status     – HTTP response status code
//   - latency    – wall-clock duration of the handler chain
//   - request_id – value from the "request_id" gin.Context key (set by RequestID)
//   - trace_id   – OpenTelemetry trace ID for log-to-trace correlation
//   - user_id    – authenticated user ID (if present)
//   - client_ip  – remote client address
//
// Responses with 5xx status are logged at Error, 4xx at Warn, rest at Debug.
func Logging(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", status),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
		}

		if rid, ok := c.Get(requestIDKey); ok {
			if s, ok := rid.(string); ok {
				fields = append(fields, zap.String("request_id", s))
			}
		}

		if tid := logging.OTelTraceIDFromContext(c.Request.Context()); tid != "" {
			fields = append(fields, zap.String("trace_id", tid))
		}

		if claims := sharedauth.ClaimsFromContext(c.Request.Context()); claims != nil {
			fields = append(fields, zap.String("user_id", claims.UserID))
		}

		switch {
		case status >= http.StatusInternalServerError:
			logger.Error("http request", fields...)
		case status >= http.StatusBadRequest:
			logger.Warn("http request", fields...)
		default:
			logger.Debug("http request", fields...)
		}
	}
}
