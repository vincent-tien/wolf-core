// response.go — Standardized HTTP response helpers for platform-level code.
// Middleware and infrastructure endpoints use these instead of raw c.JSON /
// c.AbortWithStatusJSON to guarantee a consistent error envelope across the
// entire API surface.
package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	sharederrors "github.com/vincent-tien/wolf-core/errors"
)

// ErrorResponse is the canonical JSON body for all error responses at the
// platform layer. Fields follow RFC 7807 Problem Details conventions.
type ErrorResponse struct {
	Type      string `json:"type"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Instance  string `json:"instance,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// ErrorTypeURI converts a code string to a relative URI reference for the
// RFC 7807 "type" field (e.g. "NOT_FOUND" → "/errors/not-found").
func ErrorTypeURI(code string) string {
	return "/errors/" + strings.ToLower(strings.ReplaceAll(code, "_", "-"))
}

// newErrorResponse builds an ErrorResponse for the given status, code, and message.
func newErrorResponse(status int, code, message string) ErrorResponse {
	return ErrorResponse{
		Type:    ErrorTypeURI(code),
		Status:  status,
		Code:    code,
		Message: message,
	}
}

// AbortWithError aborts the Gin handler chain and writes a structured JSON
// error response. This is the single abort point for all platform middleware.
// It auto-populates Instance from the request path and RequestID from the
// gin context (set by RequestID middleware) for consistent error responses.
func AbortWithError(c *gin.Context, status int, code, message string) {
	resp := newErrorResponse(status, code, message)
	resp.Instance = c.Request.URL.Path
	if rid, exists := c.Get("request_id"); exists {
		resp.RequestID, _ = rid.(string)
	}
	c.AbortWithStatusJSON(status, resp)
}

// AbortUnauthorized aborts with 401 Unauthorized.
func AbortUnauthorized(c *gin.Context, message string) {
	AbortWithError(c, http.StatusUnauthorized, string(sharederrors.ErrUnauthorized), message)
}

// AbortForbidden aborts with 403 Forbidden.
func AbortForbidden(c *gin.Context, message string) {
	AbortWithError(c, http.StatusForbidden, string(sharederrors.ErrForbidden), message)
}

// AbortTooManyRequests aborts with 429 Too Many Requests.
func AbortTooManyRequests(c *gin.Context, message string) {
	c.Header("Retry-After", "1")
	AbortWithError(c, http.StatusTooManyRequests, string(sharederrors.ErrRateLimited), message)
}

// AbortServiceUnavailable aborts with 503 Service Unavailable.
func AbortServiceUnavailable(c *gin.Context, message string) {
	c.Header("Retry-After", "1")
	AbortWithError(c, http.StatusServiceUnavailable, string(sharederrors.ErrUnavailable), message)
}

// AbortInternalError aborts with 500 Internal Server Error.
func AbortInternalError(c *gin.Context, message string) {
	AbortWithError(c, http.StatusInternalServerError, string(sharederrors.ErrInternal), message)
}

// AbortBadRequest aborts with 400 Bad Request.
func AbortBadRequest(c *gin.Context, message string) {
	AbortWithError(c, http.StatusBadRequest, string(sharederrors.ErrValidation), message)
}

// JSON writes a JSON response with the given status code. Platform endpoints
// (health, readiness, async) should use this instead of raw c.JSON.
func JSON(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

// Accepted writes a 202 Accepted JSON response.
func Accepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, data)
}
