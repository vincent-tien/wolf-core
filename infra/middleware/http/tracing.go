// tracing.go — OpenTelemetry distributed tracing middleware via otelgin.
package http

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// Tracing returns a Gin middleware that instruments HTTP requests with
// OpenTelemetry distributed tracing. It uses otelgin which automatically:
//   - creates a server span per request using the matched route pattern (low cardinality),
//   - propagates incoming W3C trace context from request headers,
//   - records HTTP method, status code, and route as span attributes.
//
// serviceName is embedded as the instrumentation library name in each span.
func Tracing(serviceName string) gin.HandlerFunc {
	return otelgin.Middleware(serviceName)
}
