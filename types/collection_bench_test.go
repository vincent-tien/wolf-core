// Package types_test contains benchmarks for the types package.
package types_test

import (
	"testing"

	"github.com/vincent-tien/wolf-core/types"
)

// BenchmarkMap_vs_ManualLoop compares the types.Map generic helper against a
// hand-written for-range loop over a 1 000-element int slice. The benchmark
// establishes whether the generic abstraction carries any measurable overhead
// relative to idiomatic Go code.
func BenchmarkMap_vs_ManualLoop(b *testing.B) {
	input := make([]int, 1000)
	for i := range input {
		input[i] = i
	}

	b.Run("types.Map", func(b *testing.B) {
		for range b.N {
			_ = types.Map(input, func(v int) int { return v * 2 })
		}
	})

	b.Run("manual_loop", func(b *testing.B) {
		for range b.N {
			out := make([]int, len(input))
			for i, v := range input {
				out[i] = v * 2
			}
			_ = out
		}
	})
}
