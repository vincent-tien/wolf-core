// collection.go — Generic collection helpers (Map, Filter, Reduce, etc.).
package types

import "slices"

// Map transforms each element of items using fn and returns a new slice.
// Returns a non-nil empty slice for nil input, preserving JSON serialization
// safety (nil slice marshals as JSON null; empty slice marshals as []).
func Map[T, U any](items []T, fn func(T) U) []U {
	out := make([]U, len(items))
	for i, v := range items {
		out[i] = fn(v)
	}
	return out
}

// Filter returns a new slice containing only elements for which fn returns true.
func Filter[T any](items []T, fn func(T) bool) []T {
	out := make([]T, 0, len(items))
	for _, v := range items {
		if fn(v) {
			out = append(out, v)
		}
	}
	return out
}

// Reduce folds items into a single value by applying fn to an accumulator
// starting from initial.
func Reduce[T, U any](items []T, initial U, fn func(U, T) U) U {
	acc := initial
	for _, v := range items {
		acc = fn(acc, v)
	}
	return acc
}

// GroupBy partitions items into groups keyed by the value returned by fn.
func GroupBy[T any, K comparable](items []T, fn func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, v := range items {
		k := fn(v)
		out[k] = append(out[k], v)
	}
	return out
}

// Contains reports whether v is present in items using == comparison.
func Contains[T comparable](items []T, v T) bool {
	return slices.Contains(items, v)
}

// Unique returns a new slice with duplicate elements removed, preserving the
// first occurrence order.
func Unique[T comparable](items []T) []T {
	seen := make(map[T]struct{}, len(items))
	out := make([]T, 0, len(items))
	for _, v := range items {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}
