// fake.go — Deterministic fake data generator for seeding (names, emails, etc.).
package seed

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

// Faker generates deterministic pseudo-random test data. Use NewFaker with a
// fixed seed for reproducible output, or NewRandomFaker for non-deterministic.
type Faker struct {
	rng *rand.Rand
}

// NewFaker creates a deterministic Faker seeded with the given value.
func NewFaker(seed uint64) *Faker {
	return &Faker{rng: rand.New(rand.NewPCG(seed, seed))}
}

// NewRandomFaker creates a non-deterministic Faker.
func NewRandomFaker() *Faker {
	return &Faker{rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))}
}

// Int returns a random int in [min, max].
func (f *Faker) Int(min, max int) int {
	if min >= max {
		return min
	}
	return min + f.rng.IntN(max-min+1)
}

// Float64 returns a random float64 in [min, max).
func (f *Faker) Float64(min, max float64) float64 {
	return min + f.rng.Float64()*(max-min)
}

// Bool returns a random boolean.
func (f *Faker) Bool() bool {
	return f.rng.IntN(2) == 1
}

// UUID returns a random v4-style UUID string.
func (f *Faker) UUID() string {
	var buf [16]byte
	for i := range buf {
		buf[i] = byte(f.rng.IntN(256))
	}
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// Pick returns a random element from items.
func (f *Faker) Pick(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[f.rng.IntN(len(items))]
}

// --- Personal ---

var firstNames = []string{
	"Alice", "Bob", "Charlie", "Diana", "Eve", "Frank",
	"Grace", "Henry", "Ivy", "Jack", "Karen", "Leo",
	"Mia", "Noah", "Olivia", "Paul", "Quinn", "Ruby",
	"Sam", "Tina", "Uma", "Victor", "Wendy", "Xander",
}

var lastNames = []string{
	"Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia",
	"Miller", "Davis", "Rodriguez", "Martinez", "Hernandez", "Lopez",
	"Gonzalez", "Wilson", "Anderson", "Thomas", "Taylor", "Moore",
	"Jackson", "Martin", "Lee", "Perez", "Thompson", "White",
}

// FirstName returns a random first name.
func (f *Faker) FirstName() string { return f.Pick(firstNames) }

// LastName returns a random last name.
func (f *Faker) LastName() string { return f.Pick(lastNames) }

// Name returns a random full name (first + last).
func (f *Faker) Name() string {
	return f.FirstName() + " " + f.LastName()
}

// Email returns a deterministic email using a sequence number to ensure uniqueness.
func (f *Faker) Email(seq int) string {
	first := strings.ToLower(f.FirstName())
	last := strings.ToLower(f.LastName())
	return fmt.Sprintf("%s.%s+%d@example.com", first, last, seq)
}

// --- Commerce ---

// Price returns a random price in minor units (cents) in [min, max].
func (f *Faker) Price(min, max int64) int64 {
	if min >= max {
		return min
	}
	return min + f.rng.Int64N(max-min+1)
}

// --- Text ---

var words = []string{
	"alpha", "bravo", "charlie", "delta", "echo", "foxtrot",
	"golf", "hotel", "india", "juliet", "kilo", "lima",
	"mike", "november", "oscar", "papa", "quebec", "romeo",
	"sierra", "tango", "uniform", "victor", "whiskey", "xray",
}

// Word returns a random word.
func (f *Faker) Word() string { return f.Pick(words) }

// Sentence returns a sentence of n random words.
func (f *Faker) Sentence(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = f.Word()
	}
	s := strings.Join(parts, " ")
	return strings.ToUpper(s[:1]) + s[1:] + "."
}
