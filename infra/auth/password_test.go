package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/vincent-tien/wolf-core/infra/auth"
	sharederrors "github.com/vincent-tien/wolf-core/errors"
)

func TestHashAndVerify_Success(t *testing.T) {
	// Arrange
	password := "correct-horse-Battery-staple9!"

	// Act
	hash, err := platformauth.HashPassword(password, 10)

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)

	// Verify the hash matches the original password.
	err = platformauth.VerifyPassword(hash, password)
	assert.NoError(t, err)
}

func TestVerify_WrongPassword(t *testing.T) {
	// Arrange
	password := "correct-horse-Battery-staple9!"
	hash, err := platformauth.HashPassword(password, 10)
	require.NoError(t, err)

	// Act
	err = platformauth.VerifyPassword(hash, "wrong-password")

	// Assert
	require.Error(t, err)
	var appErr *sharederrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, sharederrors.ErrValidation, appErr.Code)
}

func TestValidatePasswordStrength_AllRules(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "too short",
			password: "Ab1!",
			wantErr:  true,
			errMsg:   "at least 8 characters",
		},
		{
			name:     "no uppercase",
			password: "abcdefg1!",
			wantErr:  true,
			errMsg:   "uppercase",
		},
		{
			name:     "no lowercase",
			password: "ABCDEFG1!",
			wantErr:  true,
			errMsg:   "lowercase",
		},
		{
			name:     "no digit",
			password: "Abcdefgh!",
			wantErr:  true,
			errMsg:   "digit",
		},
		{
			name:     "no special character",
			password: "Abcdefg1",
			wantErr:  true,
			errMsg:   "special",
		},
		{
			name:     "valid password",
			password: "Correct1!",
			wantErr:  false,
		},
		{
			name:     "valid password with multiple special chars",
			password: "Horse$Battery9!",
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := platformauth.ValidatePasswordStrength(tc.password)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
