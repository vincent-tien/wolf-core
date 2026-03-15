// error_handler.go — Centralized error response middleware mapping AppError to HTTP status.
package http

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/vincent-tien/wolf-core/errors"
	wolfhttp "github.com/vincent-tien/wolf-core/infra/http"
)

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

		err := c.Errors.Last().Err
		HandleError(c, err)
	}
}

// HandleError maps err to an HTTP status code and writes a JSON error response
// to c. It is the canonical way to terminate a handler with an error.
//
// Mapping rules:
//   - apperrors.ErrNotFound      → 404
//   - apperrors.ErrConflict      → 409
//   - apperrors.ErrValidation    → 400
//   - apperrors.ErrUnauthorized  → 401
//   - apperrors.ErrForbidden     → 403
//   - apperrors.ErrInternal      → 500
//   - apperrors.ErrRateLimited   → 429
//   - apperrors.ErrUnavailable   → 503
//   - "TIMEOUT"                  → 504
//   - unknown / nil              → 500
func HandleError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	status, code, message := mapError(err)
	wolfhttp.AbortWithError(c, status, code, message)
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
		case apperrors.ErrTimeout:
			return http.StatusGatewayTimeout, string(appErr.Code), appErr.Message
		default:
			return http.StatusInternalServerError, string(appErr.Code), appErr.Message
		}
	}

	return http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred"
}
