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

// --- Required ---

func TestRequired(t *testing.T) {
	t.Parallel()

	type product struct{ Name string }

	getter := func(p product) string { return p.Name }

	tests := []struct {
		name    string
		input   product
		wantErr bool
		field   string
	}{
		{
			name:    "TestRequired_Empty",
			input:   product{Name: ""},
			wantErr: true,
			field:   "name",
		},
		{
			name:    "TestRequired_NonEmpty",
			input:   product{Name: "wolf"},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rule := validation.Required("name", getter)
			err := rule(context.Background(), tc.input)

			if tc.wantErr {
				require.Error(t, err)
				var appErr *sharedErrors.AppError
				require.ErrorAs(t, err, &appErr)
				assert.Equal(t, tc.field, appErr.Field)
				assert.Equal(t, sharedErrors.ErrValidation, appErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- MaxLength ---

func TestMaxLength(t *testing.T) {
	t.Parallel()

	type item struct{ Title string }

	getter := func(i item) string { return i.Title }

	tests := []struct {
		name    string
		max     int
		input   item
		wantErr bool
	}{
		{
			name:    "TestMaxLength_Under",
			max:     10,
			input:   item{Title: "wolf"},
			wantErr: false,
		},
		{
			name:    "TestMaxLength_Exact",
			max:     4,
			input:   item{Title: "wolf"},
			wantErr: false,
		},
		{
			name:    "TestMaxLength_Over",
			max:     3,
			input:   item{Title: "wolf"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rule := validation.MaxLength("title", tc.max, getter)
			err := rule(context.Background(), tc.input)

			if tc.wantErr {
				require.Error(t, err)
				var appErr *sharedErrors.AppError
				require.ErrorAs(t, err, &appErr)
				assert.Equal(t, "title", appErr.Field)
				assert.Equal(t, sharedErrors.ErrValidation, appErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- Positive ---

func TestPositive(t *testing.T) {
	t.Parallel()

	type order struct{ Quantity int }

	getter := func(o order) int { return o.Quantity }

	tests := []struct {
		name    string
		input   order
		wantErr bool
	}{
		{
			name:    "TestPositive_Zero",
			input:   order{Quantity: 0},
			wantErr: true,
		},
		{
			name:    "TestPositive_Negative",
			input:   order{Quantity: -5},
			wantErr: true,
		},
		{
			name:    "TestPositive_Positive",
			input:   order{Quantity: 1},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rule := validation.Positive("quantity", getter)
			err := rule(context.Background(), tc.input)

			if tc.wantErr {
				require.Error(t, err)
				var appErr *sharedErrors.AppError
				require.ErrorAs(t, err, &appErr)
				assert.Equal(t, "quantity", appErr.Field)
				assert.Equal(t, sharedErrors.ErrValidation, appErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- UniqueCheck ---

func TestUniqueCheck(t *testing.T) {
	t.Parallel()

	type user struct{ Email string }

	tests := []struct {
		name       string
		checkFn    func(ctx context.Context, u user) (bool, error)
		wantErr    bool
		errMessage string
	}{
		{
			name: "TestUniqueCheck_Exists",
			checkFn: func(_ context.Context, _ user) (bool, error) {
				return true, nil
			},
			wantErr:    true,
			errMessage: "email already exists",
		},
		{
			name: "TestUniqueCheck_NotExists",
			checkFn: func(_ context.Context, _ user) (bool, error) {
				return false, nil
			},
			wantErr: false,
		},
		{
			name: "TestUniqueCheck_Error",
			checkFn: func(_ context.Context, _ user) (bool, error) {
				return false, errors.New("db connection failed")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rule := validation.UniqueCheck("email", tc.checkFn)
			err := rule(context.Background(), user{Email: "test@example.com"})

			if tc.wantErr {
				require.Error(t, err)
				var appErr *sharedErrors.AppError
				require.ErrorAs(t, err, &appErr)
				assert.Equal(t, "email", appErr.Field)
				assert.Equal(t, sharedErrors.ErrValidation, appErr.Code)
				if tc.errMessage != "" {
					assert.Equal(t, tc.errMessage, appErr.Message)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- Custom ---

func TestCustom(t *testing.T) {
	t.Parallel()

	type payload struct{ Value string }

	tests := []struct {
		name    string
		fn      func(ctx context.Context, p payload) error
		wantErr bool
	}{
		{
			name: "TestCustom_Pass",
			fn: func(_ context.Context, _ payload) error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "TestCustom_Fail",
			fn: func(_ context.Context, _ payload) error {
				return sharedErrors.NewValidation("value", "value is invalid")
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rule := validation.Custom(tc.fn)
			err := rule(context.Background(), payload{Value: "test"})

			if tc.wantErr {
				require.Error(t, err)
				var appErr *sharedErrors.AppError
				require.ErrorAs(t, err, &appErr)
				assert.Equal(t, sharedErrors.ErrValidation, appErr.Code)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
