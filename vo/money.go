// Package vo provides immutable value objects for the wolf-be domain model.
// Value objects are identified by their attributes rather than by identity,
// and must be treated as immutable — operations return new instances.
package vo

import (
	"fmt"

	sharedErrors "github.com/vincent-tien/wolf-core/errors"
)

// Money is an immutable value object representing a monetary amount stored
// in the smallest unit of the given currency (e.g., cents for USD/EUR).
// Use NewMoney to construct instances; direct struct literal construction
// bypasses validation.
type Money struct {
	amount   int64
	currency string
}

// NewMoney constructs a validated Money value object.
// currency must be exactly 3 characters (ISO 4217), amount must be >= 0.
func NewMoney(amount int64, currency string) (Money, error) {
	if len(currency) != 3 {
		return Money{}, sharedErrors.NewValidation("currency", "currency must be a 3-character ISO 4217 code")
	}
	if amount < 0 {
		return Money{}, sharedErrors.NewValidation("amount", "amount must be >= 0")
	}
	return Money{amount: amount, currency: currency}, nil
}

// Amount returns the monetary amount in the smallest currency unit (e.g., cents).
func (m Money) Amount() int64 { return m.amount }

// Currency returns the ISO 4217 currency code (e.g., "USD", "EUR").
func (m Money) Currency() string { return m.currency }

// Add returns a new Money representing the sum of m and other.
// Returns an error if the currencies differ.
func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, fmt.Errorf("currency mismatch: cannot add %s to %s", other.currency, m.currency)
	}
	return Money{amount: m.amount + other.amount, currency: m.currency}, nil
}

// Subtract returns a new Money representing m minus other.
// Returns an error if the currencies differ or if the result would be negative.
func (m Money) Subtract(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, fmt.Errorf("currency mismatch: cannot subtract %s from %s", other.currency, m.currency)
	}
	result := m.amount - other.amount
	if result < 0 {
		return Money{}, sharedErrors.NewValidation("amount", "subtraction result must be >= 0")
	}
	return Money{amount: result, currency: m.currency}, nil
}

// Multiply returns a new Money with the amount scaled by factor.
// The currency is preserved. factor must be non-negative in typical use.
func (m Money) Multiply(factor int) Money {
	return Money{amount: m.amount * int64(factor), currency: m.currency}
}

// IsZero reports whether the monetary amount is zero.
func (m Money) IsZero() bool { return m.amount == 0 }

// Equals reports whether m and other represent the same amount and currency.
func (m Money) Equals(other Money) bool {
	return m.amount == other.amount && m.currency == other.currency
}

// String returns a human-readable representation in the form "1234 USD".
func (m Money) String() string {
	return fmt.Sprintf("%d %s", m.amount, m.currency)
}
