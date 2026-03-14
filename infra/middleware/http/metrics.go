// metrics.go — Prometheus HTTP request metrics (duration histogram, request counter).
package http

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/vincent-tien/wolf-core/infra/observability/metrics"
)

// Metrics returns a Gin middleware that records HTTP request metrics into the
// provided Prometheus collectors:
//   - HTTPRequestsInFlight: gauge incremented before and decremented after each request.
//   - HTTPRequestTotal: counter labelled by method, path (route pattern), and status code.
//   - HTTPRequestDuration: histogram labelled by method and path (route pattern).
//
// Path labels use c.FullPath() which returns the registered route pattern
// (e.g. "/api/v1/users/:id") instead of the actual URI, ensuring low
// cardinality and preventing label explosion from path parameters.
func Metrics(m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		m.HTTPRequestsInFlight.Inc()
		start := time.Now()

		c.Next()

		m.HTTPRequestsInFlight.Dec()

		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}

		elapsed := time.Since(start).Seconds()
		m.HTTPRequestTotal.WithLabelValues(method, path, status).Inc()
		m.HTTPRequestDuration.WithLabelValues(method, path, status).Observe(elapsed)
	}
}
