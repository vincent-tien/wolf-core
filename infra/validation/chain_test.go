package validation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/validation"
	sharedErrors "github.com/vincent-tien/wolf-core/errors"
)

// --- helpers ---

type testInput struct {
	Name string
}

func alwaysPass(_ context.Context, _ testInput) error { return nil }

func alwaysFail(field, msg string) validation.Rule[testInput] {
	return func(_ context.Context, _ testInput) error {
		return sharedErrors.NewValidation(field, msg)
	}
}

// --- tests ---

func TestChain_AllPass(t *testing.T) {
	t.Parallel()

	// Arrange
	chain := validation.NewChain(alwaysPass, alwaysPass)

	// Act
	err := chain.Validate(context.Background(), testInput{Name: "wolf"})

	// Assert
	assert.NoError(t, err)
}

func TestChain_SingleFailure(t *testing.T) {
	t.Parallel()

	// Arrange
	chain := validation.NewChain(
		alwaysPass,
		alwaysFail("name", "name is required"),
		alwaysPass,
	)

	// Act
	err := chain.Validate(context.Background(), testInput{})

	// Assert
	require.Error(t, err)

	var ve *validation.ValidationErrors
	require.ErrorAs(t, err, &ve)
	assert.Len(t, ve.Errors, 1)
	assert.Equal(t, "name", ve.Errors[0].Field)
	assert.Equal(t, "name is required", ve.Errors[0].Message)
}

func TestChain_MultipleFailures(t *testing.T) {
	t.Parallel()

	// Arrange — two rules both fail; chain must not short-circuit.
	chain := validation.NewChain(
		alwaysFail("name", "name is required"),
		alwaysFail("email", "email is required"),
	)

	// Act
	err := chain.Validate(context.Background(), testInput{})

	// Assert
	require.Error(t, err)

	var ve *validation.ValidationErrors
	require.ErrorAs(t, err, &ve)
	assert.Len(t, ve.Errors, 2)

	fields := []string{ve.Errors[0].Field, ve.Errors[1].Field}
	assert.Contains(t, fields, "name")
	assert.Contains(t, fields, "email")
}

func TestChain_ContextAwareRule(t *testing.T) {
	t.Parallel()

	type ctxKey string
	const key ctxKey = "skip"

	// Arrange — rule reads a context value to decide whether to fail.
	contextRule := func(ctx context.Context, _ testInput) error {
		if ctx.Value(key) == true {
			return nil
		}
		return sharedErrors.NewValidation("ctx", "context flag not set")
	}

	chain := validation.NewChain(contextRule)

	// Act — without flag: should fail.
	errWithout := chain.Validate(context.Background(), testInput{})

	// Act — with flag: should pass.
	ctx := context.WithValue(context.Background(), key, true)
	errWith := chain.Validate(ctx, testInput{})

	// Assert
	require.Error(t, errWithout)
	assert.NoError(t, errWith)
}

func TestChain_EmptyChain(t *testing.T) {
	t.Parallel()

	// Arrange
	chain := validation.NewChain[testInput]()

	// Act
	err := chain.Validate(context.Background(), testInput{})

	// Assert
	assert.NoError(t, err)
}

func TestChain_NonAppErrorIsWrapped(t *testing.T) {
	t.Parallel()

	// Arrange — rule returns a stdlib error, not an *AppError.
	plainErrRule := func(_ context.Context, _ testInput) error {
		return errors.New("plain error")
	}

	chain := validation.NewChain(plainErrRule)

	// Act
	err := chain.Validate(context.Background(), testInput{})

	// Assert
	require.Error(t, err)

	var ve *validation.ValidationErrors
	require.ErrorAs(t, err, &ve)
	assert.Len(t, ve.Errors, 1)
	assert.Equal(t, sharedErrors.ErrValidation, ve.Errors[0].Code)
	assert.Equal(t, "plain error", ve.Errors[0].Message)
}

func TestValidationErrors_Error_Single(t *testing.T) {
	t.Parallel()

	// Arrange
	ve := &validation.ValidationErrors{
		Errors: []*sharedErrors.AppError{
			sharedErrors.NewValidation("name", "name is required"),
		},
	}

	// Act + Assert
	assert.Equal(t, "name is required", ve.Error())
}

func TestValidationErrors_Error_Multiple(t *testing.T) {
	t.Parallel()

	// Arrange
	ve := &validation.ValidationErrors{
		Errors: []*sharedErrors.AppError{
			sharedErrors.NewValidation("name", "name is required"),
			sharedErrors.NewValidation("email", "email is required"),
		},
	}

	// Act + Assert
	assert.Equal(t, "validation failed: 2 error(s)", ve.Error())
}
