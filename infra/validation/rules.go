// rules.go — Composable validation rule chain for domain and application layers.
package validation

import (
	"context"
	"fmt"

	sharedErrors "github.com/vincent-tien/wolf-core/errors"
)

// Required returns a Rule that fails when getter returns an empty string.
func Required[T any](fieldName string, getter func(T) string) Rule[T] {
	return func(_ context.Context, input T) error {
		if getter(input) == "" {
			return sharedErrors.NewValidation(fieldName, fieldName+" is required")
		}
		return nil
	}
}

// MaxLength returns a Rule that fails when the length of the string returned
// by getter exceeds max characters.
func MaxLength[T any](fieldName string, max int, getter func(T) string) Rule[T] {
	return func(_ context.Context, input T) error {
		if len(getter(input)) > max {
			return sharedErrors.NewValidation(
				fieldName,
				fmt.Sprintf("%s must not exceed %d characters", fieldName, max),
			)
		}
		return nil
	}
}

// Positive returns a Rule that fails when getter returns a value that is not
// strictly greater than zero.
func Positive[T any](fieldName string, getter func(T) int) Rule[T] {
	return func(_ context.Context, input T) error {
		if getter(input) <= 0 {
			return sharedErrors.NewValidation(fieldName, fieldName+" must be a positive number")
		}
		return nil
	}
}

// Custom wraps an arbitrary validation function as a Rule. Use this for
// one-off validation logic that does not fit a standard factory.
func Custom[T any](fn func(ctx context.Context, input T) error) Rule[T] {
	return fn
}

// UniqueCheck returns a Rule that calls checkFn to determine whether a value
// already exists. checkFn must return (true, nil) when a conflict is detected,
// (false, nil) when the value is available, or (false, err) on lookup failure.
func UniqueCheck[T any](fieldName string, checkFn func(ctx context.Context, input T) (bool, error)) Rule[T] {
	return func(ctx context.Context, input T) error {
		exists, err := checkFn(ctx, input)
		if err != nil {
			return sharedErrors.NewValidation(fieldName, fmt.Sprintf("could not verify uniqueness of %s: %v", fieldName, err))
		}
		if exists {
			return sharedErrors.NewValidation(fieldName, fieldName+" already exists")
		}
		return nil
	}
}
