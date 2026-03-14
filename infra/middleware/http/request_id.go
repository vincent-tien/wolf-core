// Package http provides Gin middleware for the wolf-be HTTP layer.
// Each middleware is a standalone constructor that returns a gin.HandlerFunc,
// making them easy to compose via BuildChain or register individually.
package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/vincent-tien/wolf-core/infra/observability/logging"
)

// requestIDHeader is the canonical HTTP header used to propagate request IDs.
const requestIDHeader = "X-Request-ID"

// requestIDKey is the gin.Context key under which the request ID is stored.
const requestIDKey = "request_id"

// RequestID is a Gin middleware that ensures every request carries a unique
// identifier. If the incoming request already contains an X-Request-ID header
// its value is preserved; otherwise a new UUID v4 is generated. The ID is
// stored in gin.Context under the key "request_id" and echoed back to the
// client via the X-Request-ID response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(requestIDHeader)
		if rid == "" {
			rid = uuid.New().String()
		}
		c.Set(requestIDKey, rid)
		c.Header(requestIDHeader, rid)

		// Bridge into context.Context so use cases and repos can read it
		// via logging.RequestIDFromContext(ctx).
		ctx := logging.WithRequestID(c.Request.Context(), rid)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}
