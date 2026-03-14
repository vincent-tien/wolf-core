// password.go — Bcrypt password hashing and verification with strength validation.
package auth

import (
	"errors"
	"fmt"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	sharederrors "github.com/vincent-tien/wolf-core/errors"
)

// HashPassword hashes a plaintext password using bcrypt at the given cost.
// The returned string is the full bcrypt hash suitable for storage.
func HashPassword(password string, cost int) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword checks that password matches the given bcrypt hash.
// Returns sharederrors.NewPasswordMismatch() on any mismatch or malformed hash.
func VerifyPassword(hash, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || errors.Is(err, bcrypt.ErrHashTooShort) {
			return sharederrors.NewPasswordMismatch()
		}
		return sharederrors.NewPasswordMismatch()
	}
	return nil
}

// ValidatePasswordStrength returns an error if the password does not meet the
// minimum complexity requirements:
//   - At least 8 characters
//   - At least one uppercase letter
//   - At least one lowercase letter
//   - At least one digit
//   - At least one special (non-alphanumeric) character
func ValidatePasswordStrength(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case !unicode.IsLetter(ch) && !unicode.IsDigit(ch):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}
	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}

	return nil
}
