// Package types provides generic collection utilities for the wolf-be domain.
package types

// Set is a generic unordered collection of unique comparable values backed by
// a map[T]struct{} to guarantee O(1) membership checks.
type Set[T comparable] map[T]struct{}

// NewSet constructs a Set pre-populated with the given values. Duplicates in
// the variadic arguments are silently de-duplicated.
func NewSet[T comparable](values ...T) Set[T] {
	s := make(Set[T], len(values))
	for _, v := range values {
		s[v] = struct{}{}
	}
	return s
}

// Add inserts v into the set. No-op if v is already present.
func (s Set[T]) Add(v T) {
	s[v] = struct{}{}
}

// Remove deletes v from the set. No-op if v is not present.
func (s Set[T]) Remove(v T) {
	delete(s, v)
}

// Contains reports whether v is a member of the set.
func (s Set[T]) Contains(v T) bool {
	_, ok := s[v]
	return ok
}

// ContainsAll reports whether every item in items is a member of the set.
// Returns true for an empty items list.
func (s Set[T]) ContainsAll(items ...T) bool {
	for _, v := range items {
		if _, ok := s[v]; !ok {
			return false
		}
	}
	return true
}

// ContainsAny reports whether at least one item in items is a member of the set.
// Returns false for an empty items list.
func (s Set[T]) ContainsAny(items ...T) bool {
	for _, v := range items {
		if _, ok := s[v]; ok {
			return true
		}
	}
	return false
}

// Len returns the number of elements in the set.
func (s Set[T]) Len() int {
	return len(s)
}

// ToSlice converts the set to a slice. The order of elements is non-deterministic
// (map iteration order). The returned slice is always non-nil.
func (s Set[T]) ToSlice() []T {
	out := make([]T, 0, len(s))
	for v := range s {
		out = append(out, v)
	}
	return out
}

// Intersect returns a new Set containing only elements present in both s and other.
func (s Set[T]) Intersect(other Set[T]) Set[T] {
	// Iterate over the smaller set to minimise comparisons.
	small, large := s, other
	if len(other) < len(s) {
		small, large = other, s
	}
	result := make(Set[T], len(small))
	for v := range small {
		if _, ok := large[v]; ok {
			result[v] = struct{}{}
		}
	}
	return result
}

// Union returns a new Set containing all elements from both s and other.
func (s Set[T]) Union(other Set[T]) Set[T] {
	result := make(Set[T], len(s)+len(other))
	for v := range s {
		result[v] = struct{}{}
	}
	for v := range other {
		result[v] = struct{}{}
	}
	return result
}
