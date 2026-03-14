// Package validation provides a composable, non-short-circuiting validation
// chain for input types. All errors are collected and returned together so
// callers receive a complete picture of what failed in a single pass.
package validation

import (
	"context"
	"fmt"

	sharedErrors "github.com/vincent-tien/wolf-core/errors"
)

// Rule validates an input of type T and returns nil on success or an error
// (typically *sharedErrors.AppError) on failure.
type Rule[T any] func(ctx context.Context, input T) error

// ValidationErrors aggregates one or more *sharedErrors.AppError values
// returned by a Chain. It implements the error interface so callers can treat
// the whole collection as a single error.
type ValidationErrors struct {
	Errors []*sharedErrors.AppError
}

// Error returns a concise description of the collection.
// When exactly one error is present its message is returned directly;
// otherwise a count summary is returned.
func (ve *ValidationErrors) Error() string {
	switch len(ve.Errors) {
	case 0:
		return "validation failed: 0 error(s)"
	case 1:
		return ve.Errors[0].Message
	default:
		return fmt.Sprintf("validation failed: %d error(s)", len(ve.Errors))
	}
}

// Chain runs every registered Rule against the input, collecting all
// failures instead of stopping at the first one.
type Chain[T any] struct {
	rules []Rule[T]
}

// NewChain constructs a Chain with the given rules. The slice is copied so
// the caller cannot mutate the chain after construction.
func NewChain[T any](rules ...Rule[T]) *Chain[T] {
	r := make([]Rule[T], len(rules))
	copy(r, rules)
	return &Chain[T]{rules: r}
}

// Validate executes all rules and returns *ValidationErrors when at least one
// rule fails, or nil when all rules pass. It always evaluates every rule.
func (c *Chain[T]) Validate(ctx context.Context, input T) error {
	var errs []*sharedErrors.AppError

	for _, rule := range c.rules {
		if err := rule(ctx, input); err != nil {
			appErr := toAppError(err)
			errs = append(errs, appErr)
		}
	}

	if len(errs) == 0 {
		return nil
	}

	return &ValidationErrors{Errors: errs}
}

// toAppError converts any error into an *sharedErrors.AppError. If the error
// is already an *sharedErrors.AppError it is returned as-is; otherwise it is
// wrapped as an internal validation error so the collection stays typed.
func toAppError(err error) *sharedErrors.AppError {
	if appErr, ok := err.(*sharedErrors.AppError); ok {
		return appErr
	}
	return &sharedErrors.AppError{
		Code:    sharedErrors.ErrValidation,
		Message: err.Error(),
		Err:     err,
	}
}
