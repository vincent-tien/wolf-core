// Package vo provides immutable value objects for the wolf-be domain model.
package vo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMoney_ValidInput(t *testing.T) {
	tests := []struct {
		name     string
		amount   int64
		currency string
	}{
		{"positive amount USD", 1000, "USD"},
		{"zero amount EUR", 0, "EUR"},
		{"large amount GBP", 999999999, "GBP"},
		{"single cent JPY", 1, "JPY"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewMoney(tc.amount, tc.currency)
			require.NoError(t, err)
			assert.Equal(t, tc.amount, m.Amount())
			assert.Equal(t, tc.currency, m.Currency())
		})
	}
}

func TestNewMoney_InvalidCurrency(t *testing.T) {
	tests := []struct {
		name     string
		currency string
	}{
		{"empty currency", ""},
		{"too short", "US"},
		{"too long", "USDD"},
		{"five chars", "USDES"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewMoney(100, tc.currency)
			require.Error(t, err, "expected error for currency %q", tc.currency)
		})
	}
}

func TestNewMoney_NegativeAmount(t *testing.T) {
	_, err := NewMoney(-1, "USD")
	require.Error(t, err)
}

func TestMoney_Add_SameCurrency(t *testing.T) {
	tests := []struct {
		name       string
		aAmount    int64
		bAmount    int64
		wantAmount int64
		currency   string
	}{
		{"basic add", 1000, 500, 1500, "USD"},
		{"add zero", 1000, 0, 1000, "EUR"},
		{"both zero", 0, 0, 0, "GBP"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := NewMoney(tc.aAmount, tc.currency)
			b, _ := NewMoney(tc.bAmount, tc.currency)

			result, err := a.Add(b)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAmount, result.Amount())
			assert.Equal(t, tc.currency, result.Currency())
		})
	}
}

func TestMoney_Add_DifferentCurrency_ReturnsError(t *testing.T) {
	a, _ := NewMoney(1000, "USD")
	b, _ := NewMoney(500, "EUR")

	_, err := a.Add(b)
	require.Error(t, err)
}

func TestMoney_Subtract_SameCurrency(t *testing.T) {
	tests := []struct {
		name       string
		aAmount    int64
		bAmount    int64
		wantAmount int64
		currency   string
	}{
		{"basic subtract", 1000, 300, 700, "USD"},
		{"subtract to zero", 1000, 1000, 0, "EUR"},
		{"subtract zero", 500, 0, 500, "GBP"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := NewMoney(tc.aAmount, tc.currency)
			b, _ := NewMoney(tc.bAmount, tc.currency)

			result, err := a.Subtract(b)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAmount, result.Amount())
		})
	}
}

func TestMoney_Subtract_DifferentCurrency_ReturnsError(t *testing.T) {
	a, _ := NewMoney(1000, "USD")
	b, _ := NewMoney(200, "EUR")

	_, err := a.Subtract(b)
	require.Error(t, err)
}

func TestMoney_Subtract_NegativeResult_ReturnsError(t *testing.T) {
	a, _ := NewMoney(100, "USD")
	b, _ := NewMoney(200, "USD")

	_, err := a.Subtract(b)
	require.Error(t, err)
}

func TestMoney_Multiply(t *testing.T) {
	tests := []struct {
		name       string
		amount     int64
		factor     int
		wantAmount int64
	}{
		{"multiply by 3", 100, 3, 300},
		{"multiply by 1", 500, 1, 500},
		{"multiply by 0", 999, 0, 0},
		{"multiply large", 1000, 100, 100000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := NewMoney(tc.amount, "USD")
			result := m.Multiply(tc.factor)
			assert.Equal(t, tc.wantAmount, result.Amount())
			assert.Equal(t, "USD", result.Currency())
		})
	}
}

func TestMoney_IsZero(t *testing.T) {
	tests := []struct {
		name     string
		amount   int64
		wantZero bool
	}{
		{"zero amount", 0, true},
		{"non-zero amount", 1, false},
		{"large amount", 9999, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := NewMoney(tc.amount, "USD")
			assert.Equal(t, tc.wantZero, m.IsZero())
		})
	}
}

func TestMoney_Equals(t *testing.T) {
	tests := []struct {
		name    string
		a       Money
		b       Money
		wantEq  bool
	}{
		{
			name:   "same amount and currency",
			a:      mustMoney(t, 100, "USD"),
			b:      mustMoney(t, 100, "USD"),
			wantEq: true,
		},
		{
			name:   "different amount",
			a:      mustMoney(t, 100, "USD"),
			b:      mustMoney(t, 200, "USD"),
			wantEq: false,
		},
		{
			name:   "different currency",
			a:      mustMoney(t, 100, "USD"),
			b:      mustMoney(t, 100, "EUR"),
			wantEq: false,
		},
		{
			name:   "both zero same currency",
			a:      mustMoney(t, 0, "USD"),
			b:      mustMoney(t, 0, "USD"),
			wantEq: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantEq, tc.a.Equals(tc.b))
		})
	}
}

func TestMoney_String(t *testing.T) {
	tests := []struct {
		name     string
		amount   int64
		currency string
		want     string
	}{
		{"standard", 1234, "USD", "1234 USD"},
		{"zero", 0, "EUR", "0 EUR"},
		{"large", 100000, "GBP", "100000 GBP"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := NewMoney(tc.amount, tc.currency)
			assert.Equal(t, tc.want, m.String())
		})
	}
}

func TestMoney_Immutability_AddDoesNotMutateReceiver(t *testing.T) {
	original, _ := NewMoney(1000, "USD")
	other, _ := NewMoney(500, "USD")

	_, err := original.Add(other)
	require.NoError(t, err)

	// original must be unchanged.
	assert.Equal(t, int64(1000), original.Amount())
}

// mustMoney is a test helper that panics if NewMoney returns an error.
func mustMoney(t *testing.T, amount int64, currency string) Money {
	t.Helper()
	m, err := NewMoney(amount, currency)
	require.NoError(t, err)
	return m
}
