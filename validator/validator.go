// Package validator provides a thin wrapper around go-playground/validator/v10
// that maps struct tag validation failures to the wolf-be typed error hierarchy.
// Use Validate at API boundaries to enforce input constraints declared via
// `validate:"..."` struct tags.
package validator

import (
	"github.com/go-playground/validator/v10"
	sharedErrors "github.com/vincent-tien/wolf-core/errors"
)

// validate is the package-level validator instance, shared for efficiency.
// go-playground/validator caches struct metadata after the first use.
var validate = validator.New()

// Validate runs go-playground/validator against s and returns a typed
// sharedErrors.AppError for the first failed field, or nil if all
// constraints pass.
//
// Only struct values (or pointers to structs) should be passed. Passing a
// non-struct value results in a validation error from the underlying library.
func Validate(s interface{}) error {
	if err := validate.Struct(s); err != nil {
		var ve validator.ValidationErrors
		if ok := isValidationErrors(err, &ve); ok && len(ve) > 0 {
			first := ve[0]
			return sharedErrors.NewValidation(
				first.Field(),
				buildMessage(first),
			)
		}
		// Structural error (non-struct input) — treat as internal validation failure.
		return sharedErrors.NewValidation("input", err.Error())
	}
	return nil
}

// isValidationErrors is a helper that avoids importing "errors" directly.
func isValidationErrors(err error, target *validator.ValidationErrors) bool {
	if ve, ok := err.(validator.ValidationErrors); ok {
		*target = ve
		return true
	}
	return false
}

// buildMessage constructs a human-readable validation message from a
// single FieldError returned by go-playground/validator.
func buildMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return fe.Field() + " is required"
	case "min":
		return fe.Field() + " must be at least " + fe.Param()
	case "max":
		return fe.Field() + " must be at most " + fe.Param()
	case "email":
		return fe.Field() + " must be a valid email address"
	case "uuid", "uuid4":
		return fe.Field() + " must be a valid UUID"
	case "oneof":
		return fe.Field() + " must be one of: " + fe.Param()
	default:
		return fe.Field() + " failed validation: " + fe.Tag()
	}
}
