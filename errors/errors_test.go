// Package errors provides a typed application error hierarchy for the wolf-be platform.
package errors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- Constructor tests ---

func TestNewNotFound_ErrorString(t *testing.T) {
	err := NewNotFound("Order", "order-123")
	assert.Equal(t, ErrNotFound, err.Code)
	assert.Contains(t, err.Error(), "Order")
	assert.Contains(t, err.Error(), "order-123")
}

func TestNewConflict_ErrorString(t *testing.T) {
	err := NewConflict("email already in use")
	assert.Equal(t, ErrConflict, err.Code)
	assert.Contains(t, err.Error(), "email already in use")
}

func TestNewValidation_IncludesField(t *testing.T) {
	err := NewValidation("email", "must be a valid email")
	assert.Equal(t, ErrValidation, err.Code)
	assert.Equal(t, "email", err.Field)
	assert.Contains(t, err.Error(), "email")
	assert.Contains(t, err.Error(), "must be a valid email")
}

func TestNewUnauthorized_ErrorString(t *testing.T) {
	err := NewUnauthorized("token expired")
	assert.Equal(t, ErrUnauthorized, err.Code)
	assert.Contains(t, err.Error(), "token expired")
}

func TestNewForbidden_ErrorString(t *testing.T) {
	err := NewForbidden("insufficient permissions")
	assert.Equal(t, ErrForbidden, err.Code)
	assert.Contains(t, err.Error(), "insufficient permissions")
}

func TestNewInternal_WrapsUnderlyingError(t *testing.T) {
	cause := errors.New("db connection refused")
	err := NewInternal(cause)
	assert.Equal(t, ErrInternal, err.Code)
	assert.Equal(t, cause, errors.Unwrap(err))
	assert.Contains(t, err.Error(), "internal server error")
}

func TestNewRateLimited_ErrorString(t *testing.T) {
	err := NewRateLimited()
	assert.Equal(t, ErrRateLimited, err.Code)
	assert.Contains(t, err.Error(), "too many requests")
}

func TestNewUnavailable_ErrorString(t *testing.T) {
	err := NewUnavailable("payment gateway unavailable")
	assert.Equal(t, ErrUnavailable, err.Code)
	assert.Contains(t, err.Error(), "payment gateway unavailable")
}

// --- Error.Error() format tests ---

func TestAppError_Error_WithFieldAndCause(t *testing.T) {
	cause := errors.New("underlying")
	err := &AppError{Code: ErrValidation, Message: "bad input", Field: "name", Err: cause}
	msg := err.Error()
	assert.Contains(t, msg, "VALIDATION")
	assert.Contains(t, msg, "name")
	assert.Contains(t, msg, "bad input")
	assert.Contains(t, msg, "underlying")
}

func TestAppError_Error_WithFieldNoErr(t *testing.T) {
	err := &AppError{Code: ErrValidation, Message: "bad input", Field: "name"}
	msg := err.Error()
	assert.Contains(t, msg, "VALIDATION")
	assert.Contains(t, msg, "name")
	assert.Contains(t, msg, "bad input")
}

func TestAppError_Error_NoCauseNoField(t *testing.T) {
	err := &AppError{Code: ErrNotFound, Message: "not found"}
	msg := err.Error()
	assert.Contains(t, msg, "NOT_FOUND")
	assert.Contains(t, msg, "not found")
}

func TestAppError_Error_WithCauseNoField(t *testing.T) {
	cause := errors.New("root cause")
	err := &AppError{Code: ErrInternal, Message: "server error", Err: cause}
	msg := err.Error()
	assert.Contains(t, msg, "INTERNAL")
	assert.Contains(t, msg, "server error")
	assert.Contains(t, msg, "root cause")
}

// --- Unwrap tests ---

func TestAppError_Unwrap(t *testing.T) {
	cause := errors.New("original")
	err := NewInternal(cause)
	assert.Equal(t, cause, errors.Unwrap(err))
}

func TestAppError_Unwrap_Nil(t *testing.T) {
	err := NewNotFound("Order", "1")
	assert.Nil(t, errors.Unwrap(err))
}

// --- errors.Is tests ---

func TestAppError_Is_SameCode(t *testing.T) {
	err := NewNotFound("Order", "1")
	target := &AppError{Code: ErrNotFound}
	assert.True(t, errors.Is(err, target))
}

func TestAppError_Is_DifferentCode(t *testing.T) {
	err := NewNotFound("Order", "1")
	target := &AppError{Code: ErrConflict}
	assert.False(t, errors.Is(err, target))
}

func TestAppError_Is_WrappedInFmtErrorf(t *testing.T) {
	inner := NewNotFound("User", "u-1")
	wrapped := fmt.Errorf("repo layer: %w", inner)
	assert.True(t, errors.Is(wrapped, sentinelNotFound))
}

// --- Helper predicate tests ---

func TestIsNotFound(t *testing.T) {
	assert.True(t, IsNotFound(NewNotFound("X", "1")))
	assert.False(t, IsNotFound(NewConflict("conflict")))
	assert.False(t, IsNotFound(errors.New("plain error")))
}

func TestIsConflict(t *testing.T) {
	assert.True(t, IsConflict(NewConflict("dup")))
	assert.False(t, IsConflict(NewNotFound("X", "1")))
}

func TestIsValidation(t *testing.T) {
	assert.True(t, IsValidation(NewValidation("field", "msg")))
	assert.False(t, IsValidation(NewInternal(errors.New("x"))))
}

func TestIsUnauthorized(t *testing.T) {
	assert.True(t, IsUnauthorized(NewUnauthorized("msg")))
	assert.False(t, IsUnauthorized(NewForbidden("msg")))
}

func TestIsForbidden(t *testing.T) {
	assert.True(t, IsForbidden(NewForbidden("msg")))
	assert.False(t, IsForbidden(NewUnauthorized("msg")))
}

func TestIsInternal(t *testing.T) {
	assert.True(t, IsInternal(NewInternal(errors.New("cause"))))
	assert.False(t, IsInternal(NewNotFound("X", "1")))
}

func TestIsRateLimited(t *testing.T) {
	assert.True(t, IsRateLimited(NewRateLimited()))
	assert.False(t, IsRateLimited(NewInternal(errors.New("x"))))
}

func TestIsUnavailable(t *testing.T) {
	assert.True(t, IsUnavailable(NewUnavailable("down")))
	assert.False(t, IsUnavailable(NewNotFound("X", "1")))
}

func TestIsHelpers_WrappedErrors(t *testing.T) {
	inner := NewForbidden("no access")
	wrapped := fmt.Errorf("handler: %w", inner)
	assert.True(t, IsForbidden(wrapped))
}

// --- All error codes have distinct values ---

func TestErrorCodes_AreDistinct(t *testing.T) {
	codes := []ErrorCode{
		ErrNotFound, ErrConflict, ErrValidation,
		ErrUnauthorized, ErrForbidden, ErrInternal,
		ErrRateLimited, ErrUnavailable,
	}
	seen := make(map[ErrorCode]struct{})
	for _, c := range codes {
		_, dup := seen[c]
		assert.False(t, dup, "duplicate error code: %q", c)
		seen[c] = struct{}{}
	}
}
