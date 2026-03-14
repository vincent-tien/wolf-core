// validation.go — Messenger middleware that validates messages before dispatch.
package middleware

import (
	"context"
	"fmt"

	"github.com/vincent-tien/wolf-core/messenger"
)

// ValidateFunc validates a message struct before dispatch.
type ValidateFunc func(msg any) error

// Validation validates message structs before handler execution.
type Validation struct {
	validate ValidateFunc
}

// NewValidation creates a validation middleware with the given validator function.
func NewValidation(validate ValidateFunc) *Validation {
	return &Validation{validate: validate}
}

func (m *Validation) Handle(ctx context.Context, env messenger.Envelope, next messenger.MiddlewareNext) (messenger.DispatchResult, error) {
	if err := m.validate(env.Message); err != nil {
		return messenger.DispatchResult{}, fmt.Errorf("messenger: validation failed for %s: %w", env.MessageTypeName(), err)
	}
	return next(ctx, env)
}
