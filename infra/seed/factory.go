// factory.go — Generic test data factory with sequence-based unique values.
package seed

import (
	"maps"
	"sync/atomic"
)

// Definition is a function that returns a default instance of T.
// The index i (0-based) and Faker are provided for generating varied data.
type Definition[T any] func(i int, f *Faker) T

// State modifies an instance to represent a named variant (e.g. "inactive").
type State[T any] struct {
	Name  string
	Apply func(t *T, i int, f *Faker)
}

// Factory builds typed instances of T. It is immutable: chain methods return
// a new Factory leaving the original unchanged.
type Factory[T any] struct {
	def    Definition[T]
	states map[string]State[T]
	faker  *Faker
	idx    int
}

// NewFactory creates a Factory with the given default definition.
func NewFactory[T any](def Definition[T]) *Factory[T] {
	return &Factory[T]{
		def:    def,
		states: make(map[string]State[T]),
		faker:  NewFaker(42),
	}
}

// WithState returns a new Factory that knows about the given state.
func (f *Factory[T]) WithState(s State[T]) *Factory[T] {
	newStates := make(map[string]State[T], len(f.states)+1)
	maps.Copy(newStates, f.states)
	newStates[s.Name] = s
	return &Factory[T]{
		def:    f.def,
		states: newStates,
		faker:  f.faker,
		idx:    f.idx,
	}
}

// WithFaker returns a new Factory using the given Faker instance.
func (f *Factory[T]) WithFaker(faker *Faker) *Factory[T] {
	return &Factory[T]{
		def:    f.def,
		states: f.states,
		faker:  faker,
		idx:    f.idx,
	}
}

// Make creates a single instance, optionally applying named states.
func (f *Factory[T]) Make(states ...string) T {
	instance := f.def(f.idx, f.faker)
	f.idx++
	for _, name := range states {
		if s, ok := f.states[name]; ok {
			s.Apply(&instance, f.idx-1, f.faker)
		}
	}
	return instance
}

// MakeMany creates n instances, optionally applying named states to each.
func (f *Factory[T]) MakeMany(n int, states ...string) []T {
	result := make([]T, n)
	for i := range n {
		result[i] = f.Make(states...)
	}
	return result
}

// MakeWithOverride creates a single instance and applies a custom override
// function after any named states.
func (f *Factory[T]) MakeWithOverride(override func(*T), states ...string) T {
	instance := f.Make(states...)
	override(&instance)
	return instance
}

// Sequence generates incrementing values of type T.
type Sequence[T any] struct {
	counter atomic.Int64
	gen     func(n int64) T
}

// NewSequence creates a sequence starting at 1.
func NewSequence[T any](gen func(n int64) T) *Sequence[T] {
	return &Sequence[T]{gen: gen}
}

// Next returns the next value in the sequence.
func (s *Sequence[T]) Next() T {
	n := s.counter.Add(1)
	return s.gen(n)
}
