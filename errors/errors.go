// Package errors provides a typed application error hierarchy for the wolf-be
// platform. All layers should wrap lower-level errors using the constructors
// in this package to ensure consistent HTTP status mapping and client responses.
package errors

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorCode identifies the semantic category of an application error.
// Each code maps to a specific HTTP status in the transport layer.
type ErrorCode string

const (
	// ErrNotFound indicates that a requested resource does not exist.
	ErrNotFound ErrorCode = "NOT_FOUND"
	// ErrConflict indicates a state conflict, such as a duplicate resource.
	ErrConflict ErrorCode = "CONFLICT"
	// ErrValidation indicates that input failed structural or business validation.
	ErrValidation ErrorCode = "VALIDATION"
	// ErrUnauthorized indicates that authentication is missing or invalid.
	ErrUnauthorized ErrorCode = "UNAUTHORIZED"
	// ErrForbidden indicates that the caller lacks permission for the operation.
	ErrForbidden ErrorCode = "FORBIDDEN"
	// ErrInternal indicates an unexpected server-side failure.
	ErrInternal ErrorCode = "INTERNAL"
	// ErrRateLimited indicates the caller has exceeded their request quota.
	ErrRateLimited ErrorCode = "RATE_LIMITED"
	// ErrUnavailable indicates that a downstream dependency is temporarily unavailable.
	ErrUnavailable ErrorCode = "UNAVAILABLE"
	// ErrTimeout indicates that an operation exceeded its deadline.
	ErrTimeout ErrorCode = "TIMEOUT"
)

// AppError is the canonical application error type. It carries a semantic code,
// a human-readable message, an optional field name for validation errors, and
// an optional wrapped cause for error chain inspection.
type AppError struct {
	// Code identifies the error category for programmatic handling.
	Code ErrorCode
	// Message is the human-readable description of the error.
	Message string
	// Field identifies the invalid field for validation errors; empty otherwise.
	Field string
	// Err is the underlying cause, available via errors.Unwrap.
	Err error
}

// Error implements the error interface, returning a formatted string that
// includes the code, optional field, message, and cause when present.
func (e *AppError) Error() string {
	if e.Field != "" {
		if e.Err != nil {
			return fmt.Sprintf("[%s] %s: %s: %v", e.Code, e.Field, e.Message, e.Err)
		}
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Field, e.Message)
	}
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the wrapped cause error, enabling errors.Is / errors.As traversal.
func (e *AppError) Unwrap() error { return e.Err }

// Is reports whether the target error is an AppError with the same Code.
// This allows errors.Is(err, &AppError{Code: ErrNotFound}) comparisons.
func (e *AppError) Is(target error) bool {
	var t *AppError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// --- Sentinel errors for errors.Is matching ---

// sentinelNotFound is used as target in IsNotFound checks.
var sentinelNotFound = &AppError{Code: ErrNotFound}

// sentinelConflict is used as target in IsConflict checks.
var sentinelConflict = &AppError{Code: ErrConflict}

// sentinelValidation is used as target in IsValidation checks.
var sentinelValidation = &AppError{Code: ErrValidation}

// sentinelUnauthorized is used as target in IsUnauthorized checks.
var sentinelUnauthorized = &AppError{Code: ErrUnauthorized}

// sentinelForbidden is used as target in IsForbidden checks.
var sentinelForbidden = &AppError{Code: ErrForbidden}

// sentinelInternal is used as target in IsInternal checks.
var sentinelInternal = &AppError{Code: ErrInternal}

// sentinelRateLimited is used as target in IsRateLimited checks.
var sentinelRateLimited = &AppError{Code: ErrRateLimited}

// sentinelUnavailable is used as target in IsUnavailable checks.
var sentinelUnavailable = &AppError{Code: ErrUnavailable}

// --- Constructors ---

// NewNotFound returns an ErrNotFound AppError for the given entity type and ID.
func NewNotFound(entity, id string) *AppError {
	return &AppError{
		Code:    ErrNotFound,
		Message: fmt.Sprintf("%s with id %q not found", entity, id),
	}
}

// NewConflict returns an ErrConflict AppError with the given message.
func NewConflict(msg string) *AppError {
	return &AppError{
		Code:    ErrConflict,
		Message: msg,
	}
}

// NewValidation returns an ErrValidation AppError for the given field and message.
func NewValidation(field, msg string) *AppError {
	return &AppError{
		Code:    ErrValidation,
		Message: msg,
		Field:   field,
	}
}

// NewUnauthorized returns an ErrUnauthorized AppError with the given message.
func NewUnauthorized(msg string) *AppError {
	return &AppError{
		Code:    ErrUnauthorized,
		Message: msg,
	}
}

// NewForbidden returns an ErrForbidden AppError with the given message.
func NewForbidden(msg string) *AppError {
	return &AppError{
		Code:    ErrForbidden,
		Message: msg,
	}
}

// NewInternal returns an ErrInternal AppError wrapping the given cause.
// The cause is available via errors.Unwrap for server-side logging.
func NewInternal(err error) *AppError {
	return &AppError{
		Code:    ErrInternal,
		Message: "an internal server error occurred",
		Err:     err,
	}
}

// NewRateLimited returns an ErrRateLimited AppError with a standard message.
func NewRateLimited() *AppError {
	return &AppError{
		Code:    ErrRateLimited,
		Message: "too many requests, please slow down",
	}
}

// NewUnavailable returns an ErrUnavailable AppError with the given message.
func NewUnavailable(msg string) *AppError {
	return &AppError{
		Code:    ErrUnavailable,
		Message: msg,
	}
}

// --- Auth-domain constructors ---
//
// These constructors wrap generic error codes with auth-specific messages.
// They live in shared/errors (not platform/auth) because both platform
// middleware and business modules need them, and the dependency direction
// (shared ← platform ← modules) prevents a clean single-owner move.
// If reusing this template for a non-auth project, remove this section.

// NewTokenExpired returns an unauthorized error for expired tokens.
func NewTokenExpired() *AppError {
	return &AppError{Code: ErrUnauthorized, Message: "token has expired"}
}

// NewTokenInvalid returns an unauthorized error for malformed/invalid tokens.
func NewTokenInvalid(reason string) *AppError {
	return &AppError{Code: ErrUnauthorized, Message: fmt.Sprintf("invalid token: %s", reason)}
}

// NewTokenRevoked returns an unauthorized error for revoked/blacklisted tokens.
func NewTokenRevoked() *AppError {
	return &AppError{Code: ErrUnauthorized, Message: "token has been revoked"}
}

// NewInsufficientRole returns a forbidden error when the user lacks required roles.
func NewInsufficientRole(required []string) *AppError {
	return &AppError{
		Code:    ErrForbidden,
		Message: fmt.Sprintf("requires one of roles: %s", strings.Join(required, ", ")),
	}
}

// NewInsufficientPermission returns a forbidden error when the user lacks a permission.
func NewInsufficientPermission(perm string) *AppError {
	return &AppError{Code: ErrForbidden, Message: fmt.Sprintf("missing permission: %s", perm)}
}

// NewPasswordMismatch returns a validation error for incorrect password.
func NewPasswordMismatch() *AppError {
	return &AppError{Code: ErrValidation, Message: "incorrect email or password"}
}

// --- Helper predicates ---

// IsNotFound reports whether err is (or wraps) an ErrNotFound AppError.
func IsNotFound(err error) bool { return errors.Is(err, sentinelNotFound) }

// IsConflict reports whether err is (or wraps) an ErrConflict AppError.
func IsConflict(err error) bool { return errors.Is(err, sentinelConflict) }

// IsValidation reports whether err is (or wraps) an ErrValidation AppError.
func IsValidation(err error) bool { return errors.Is(err, sentinelValidation) }

// IsUnauthorized reports whether err is (or wraps) an ErrUnauthorized AppError.
func IsUnauthorized(err error) bool { return errors.Is(err, sentinelUnauthorized) }

// IsForbidden reports whether err is (or wraps) an ErrForbidden AppError.
func IsForbidden(err error) bool { return errors.Is(err, sentinelForbidden) }

// IsInternal reports whether err is (or wraps) an ErrInternal AppError.
func IsInternal(err error) bool { return errors.Is(err, sentinelInternal) }

// IsRateLimited reports whether err is (or wraps) an ErrRateLimited AppError.
func IsRateLimited(err error) bool { return errors.Is(err, sentinelRateLimited) }

// IsUnavailable reports whether err is (or wraps) an ErrUnavailable AppError.
func IsUnavailable(err error) bool { return errors.Is(err, sentinelUnavailable) }
