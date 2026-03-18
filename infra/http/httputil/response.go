// Package httputil provides standardised HTTP response helpers for Gin handlers.
// All handlers MUST use these helpers instead of calling c.JSON directly to
// guarantee a consistent response envelope across the API.
//
// The default ErrorBody includes common RFC 7807 fields. Apps that need a
// different error shape can call Configure(WithErrorMapper(fn)) once at startup.
package httputil

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	sharederrors "github.com/vincent-tien/wolf-core/errors"
)

// Response is the standard JSON response envelope used by every endpoint.
type Response struct {
	Data  any `json:"data,omitempty"`
	Error any `json:"error,omitempty"`
}

// ErrorBody is the default error body following RFC 7807 Problem Details
// conventions. Apps needing a different shape should supply a custom
// ErrorMapper via Configure(WithErrorMapper(fn)).
type ErrorBody struct {
	Type     string `json:"type"`
	Status   int    `json:"status"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Instance string `json:"instance,omitempty"`
	Field    string `json:"field,omitempty"`
}

// ErrorMapper converts an error into an HTTP status code and a
// JSON-serializable error body. The body is placed in Response.Error.
type ErrorMapper func(c *gin.Context, err error) (status int, body any)

// Option configures a Responder.
type Option func(*Responder)

// WithErrorMapper sets a custom error mapper replacing DefaultErrorMapper.
func WithErrorMapper(m ErrorMapper) Option {
	return func(r *Responder) { r.errorMapper = m }
}

// Responder provides standardised HTTP response methods. Use NewResponder to
// create one with custom options, or use the package-level functions which
// delegate to the Default instance.
type Responder struct {
	errorMapper ErrorMapper
}

// NewResponder creates a Responder with the given options.
func NewResponder(opts ...Option) *Responder {
	r := &Responder{errorMapper: DefaultErrorMapper}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Default is the package-level Responder used by OK, Created, Error, etc.
var Default = NewResponder()

// Configure replaces the Default Responder. Call once at startup before
// handling requests.
func Configure(opts ...Option) {
	Default = NewResponder(opts...)
}

// --- Package-level convenience functions (delegate to Default) ---

func OK(c *gin.Context, data any)      { Default.OK(c, data) }
func Created(c *gin.Context, data any)  { Default.Created(c, data) }
func NoContent(c *gin.Context)          { Default.NoContent(c) }
func Error(c *gin.Context, err error)   { Default.Error(c, err) }

// --- Responder methods ---

func (r *Responder) OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Data: data})
}

func (r *Responder) Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Response{Data: data})
}

func (r *Responder) NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func (r *Responder) Error(c *gin.Context, err error) {
	status, body := r.errorMapper(c, err)
	c.JSON(status, Response{Error: body})
}

// --- Default error mapper ---

// DefaultErrorMapper maps errors to the standard ErrorBody. AppErrors are
// translated to their corresponding HTTP status; all other errors produce 500.
func DefaultErrorMapper(c *gin.Context, err error) (int, any) {
	var appErr *sharederrors.AppError
	if !errors.As(err, &appErr) {
		status := http.StatusInternalServerError
		return status, &ErrorBody{
			Type:     ErrorTypeURI(string(sharederrors.ErrInternal)),
			Status:   status,
			Code:     string(sharederrors.ErrInternal),
			Message:  "an internal server error occurred",
			Instance: c.Request.URL.Path,
		}
	}

	status := CodeToStatus(appErr.Code)
	return status, &ErrorBody{
		Type:     ErrorTypeURI(string(appErr.Code)),
		Status:   status,
		Code:     string(appErr.Code),
		Message:  appErr.Message,
		Instance: c.Request.URL.Path,
		Field:    appErr.Field,
	}
}

// --- Shared helpers (exported for custom ErrorMapper implementations) ---

// ErrorTypeURI converts an error code string to a relative URI reference
// for the RFC 7807 "type" field (e.g. "NOT_FOUND" → "/errors/not-found").
func ErrorTypeURI(code string) string {
	return "/errors/" + strings.ToLower(strings.ReplaceAll(code, "_", "-"))
}

// CodeToStatus maps a sharederrors.ErrorCode to an HTTP status code.
func CodeToStatus(code sharederrors.ErrorCode) int {
	switch code {
	case sharederrors.ErrNotFound:
		return http.StatusNotFound
	case sharederrors.ErrValidation:
		return http.StatusBadRequest
	case sharederrors.ErrConflict:
		return http.StatusConflict
	case sharederrors.ErrUnauthorized:
		return http.StatusUnauthorized
	case sharederrors.ErrForbidden:
		return http.StatusForbidden
	case sharederrors.ErrRateLimited:
		return http.StatusTooManyRequests
	case sharederrors.ErrUnavailable:
		return http.StatusServiceUnavailable
	case sharederrors.ErrTimeout:
		return http.StatusGatewayTimeout
	case sharederrors.ErrInternal:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}
