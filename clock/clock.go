// Package clock provides a time abstraction that enables deterministic testing
// of time-dependent logic. Production code receives a RealClock; test code
// receives a FakeClock with a fixed instant.
package clock

import "time"

// Clock abstracts the system clock so that time-dependent code can be tested
// without relying on the wall clock.
type Clock interface {
	// Now returns the current time. Implementations must be safe for concurrent use.
	Now() time.Time
}

// RealClock is the production Clock implementation. It delegates to time.Now().
type RealClock struct{}

// Now returns the current wall-clock time via time.Now().
func (RealClock) Now() time.Time { return time.Now() }

// FakeClock is a deterministic Clock for use in unit tests. It always returns
// the value of Fixed, which can be set directly by the test.
type FakeClock struct {
	// Fixed is the time value returned by Now. Set this field to control
	// the apparent current time in tests.
	Fixed time.Time
}

// Now returns the fixed time configured on the FakeClock.
func (f FakeClock) Now() time.Time { return f.Fixed }
