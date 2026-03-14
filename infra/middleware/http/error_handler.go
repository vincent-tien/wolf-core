// error_handler.go — Centralized error response middleware mapping AppError to HTTP status.
package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/vincent-tien/wolf-core/errors"
)

// errorResponse is the canonical JSON body returned for all error responses.
// Fields follow RFC 7807 Problem Details conventions while retaining backward
// compatibility via Code/Message.
type errorResponse struct {
	Type      string `json:"type"`               // URI reference identifying the error category
	Status    int    `json:"status"`             // HTTP status code
	Code      string `json:"code"`               // Machine-readable error code
	Message   string `json:"message"`            // Human-readable error detail
	Instance  string `json:"instance,omitempty"` // Request path
	RequestID string `json:"request_id,omitempty"`
}

// ErrorHandler returns a Gin middleware that checks gin.Context.Errors after
// the handler chain completes. If any errors were attached to the context via
// c.Error(), the last one is mapped to an HTTP status code and written as a
// JSON error response. Requests with no errors pass through unchanged.
//
// Use HandleError for explicit one-shot error responses from within handlers.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		// Use the last error attached to the context.
		err := c.Errors.Last().Err
		HandleError(c, err)
	}
}

// HandleError maps err to an HTTP status code and writes a JSON error response
// to c. It is the canonical way to terminate a handler with an error.
//
// Mapping rules:
//   - customerror.ErrorTypeNotFound      → 404
//   - customerror.ErrorTypeConflict      → 409
//   - customerror.ErrorTypeValidation    → 400
//   - customerror.ErrorTypeUnauthorized  → 401
//   - customerror.ErrorTypeForbidden     → 403
//   - customerror.ErrorTypeInternal      → 500
//   - unknown / nil                      → 500
//
// Additional error codes not present in customerror.ErrorType are detected by
// code string matching for forward-compatibility:
//   - code "RATE_LIMITED"  → 429
//   - code "TIMEOUT"       → 504
//   - code "UNAVAILABLE"   → 503
func HandleError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	status, code, message := mapError(err)

	rid, _ := c.Get(requestIDKey)
	ridStr, _ := rid.(string)

	c.AbortWithStatusJSON(status, errorResponse{
		Type:      errorTypeURI(code),
		Status:    status,
		Code:      code,
		Message:   message,
		Instance:  c.Request.URL.Path,
		RequestID: ridStr,
	})
}

// errorTypeURI converts a code string to a relative URI reference for the
// RFC 7807 "type" field (e.g. "NOT_FOUND" → "/errors/not-found").
func errorTypeURI(code string) string {
	return "/errors/" + strings.ToLower(strings.ReplaceAll(code, "_", "-"))
}

// mapError resolves an error into an HTTP status code, machine-readable code
// string, and human-readable message.
func mapError(err error) (status int, code, message string) {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case apperrors.ErrNotFound:
			return http.StatusNotFound, string(appErr.Code), appErr.Message
		case apperrors.ErrConflict:
			return http.StatusConflict, string(appErr.Code), appErr.Message
		case apperrors.ErrValidation:
			return http.StatusBadRequest, string(appErr.Code), appErr.Message
		case apperrors.ErrUnauthorized:
			return http.StatusUnauthorized, string(appErr.Code), appErr.Message
		case apperrors.ErrForbidden:
			return http.StatusForbidden, string(appErr.Code), appErr.Message
		case apperrors.ErrInternal:
			return http.StatusInternalServerError, string(appErr.Code), appErr.Message
		case apperrors.ErrRateLimited:
			return http.StatusTooManyRequests, string(appErr.Code), appErr.Message
		case apperrors.ErrUnavailable:
			return http.StatusServiceUnavailable, string(appErr.Code), appErr.Message
		case "TIMEOUT":
			return http.StatusGatewayTimeout, string(appErr.Code), appErr.Message
		default:
			return http.StatusInternalServerError, string(appErr.Code), appErr.Message
		}
	}

	return http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred"
}
