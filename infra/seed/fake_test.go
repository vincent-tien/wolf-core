package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFaker_Deterministic(t *testing.T) {
	f1 := NewFaker(42)
	f2 := NewFaker(42)

	// Same seed produces same sequence.
	for range 10 {
		assert.Equal(t, f1.Int(0, 100), f2.Int(0, 100))
	}
}

func TestFaker_DifferentSeeds(t *testing.T) {
	f1 := NewFaker(1)
	f2 := NewFaker(2)

	// Different seeds produce different sequences (with high probability).
	same := 0
	for range 20 {
		if f1.Int(0, 1000) == f2.Int(0, 1000) {
			same++
		}
	}
	assert.Less(t, same, 15, "different seeds should produce mostly different values")
}

func TestFaker_Int_Range(t *testing.T) {
	f := NewFaker(42)
	for range 100 {
		v := f.Int(5, 10)
		assert.GreaterOrEqual(t, v, 5)
		assert.LessOrEqual(t, v, 10)
	}
}

func TestFaker_Int_EqualMinMax(t *testing.T) {
	f := NewFaker(42)
	assert.Equal(t, 5, f.Int(5, 5))
}

func TestFaker_Float64_Range(t *testing.T) {
	f := NewFaker(42)
	for range 100 {
		v := f.Float64(1.0, 10.0)
		assert.GreaterOrEqual(t, v, 1.0)
		assert.Less(t, v, 10.0)
	}
}

func TestFaker_Bool(t *testing.T) {
	f := NewFaker(42)
	trueCount := 0
	for range 100 {
		if f.Bool() {
			trueCount++
		}
	}
	// Should have a reasonable distribution.
	assert.Greater(t, trueCount, 10)
	assert.Less(t, trueCount, 90)
}

func TestFaker_UUID_Format(t *testing.T) {
	f := NewFaker(42)
	uuid := f.UUID()

	assert.Len(t, uuid, 36, "UUID should be 36 chars (8-4-4-4-12)")
	assert.Equal(t, byte('-'), uuid[8])
	assert.Equal(t, byte('-'), uuid[13])
	assert.Equal(t, byte('-'), uuid[18])
	assert.Equal(t, byte('-'), uuid[23])
}

func TestFaker_UUID_Deterministic(t *testing.T) {
	f1 := NewFaker(42)
	f2 := NewFaker(42)
	assert.Equal(t, f1.UUID(), f2.UUID())
}

func TestFaker_Pick(t *testing.T) {
	f := NewFaker(42)
	items := []string{"a", "b", "c"}

	for range 50 {
		v := f.Pick(items)
		assert.Contains(t, items, v)
	}
}

func TestFaker_Pick_Empty(t *testing.T) {
	f := NewFaker(42)
	assert.Equal(t, "", f.Pick(nil))
	assert.Equal(t, "", f.Pick([]string{}))
}

func TestFaker_Name(t *testing.T) {
	f := NewFaker(42)
	name := f.Name()
	require.NotEmpty(t, name)
	assert.Contains(t, name, " ", "full name should have a space")
}

func TestFaker_Email(t *testing.T) {
	f := NewFaker(42)
	email := f.Email(1)
	assert.Contains(t, email, "@example.com")
	assert.Contains(t, email, "+1@")
}

func TestFaker_Price_Range(t *testing.T) {
	f := NewFaker(42)
	for range 50 {
		p := f.Price(100, 10000)
		assert.GreaterOrEqual(t, p, int64(100))
		assert.LessOrEqual(t, p, int64(10000))
	}
}

func TestFaker_Sentence(t *testing.T) {
	f := NewFaker(42)
	s := f.Sentence(5)
	assert.NotEmpty(t, s)
	// Should end with a period.
	assert.Equal(t, byte('.'), s[len(s)-1])
	// First letter capitalized.
	assert.True(t, s[0] >= 'A' && s[0] <= 'Z')
}

func TestFaker_Sentence_Zero(t *testing.T) {
	f := NewFaker(42)
	assert.Equal(t, "", f.Sentence(0))
	assert.Equal(t, "", f.Sentence(-1))
}
