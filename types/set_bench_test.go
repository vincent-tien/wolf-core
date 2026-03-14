// Package types_test contains benchmarks for the types package.
package types_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/vincent-tien/wolf-core/types"
)

// BenchmarkSet_Contains measures O(1) map-based membership checks at various
// collection sizes. Compare with BenchmarkSlice_Contains to quantify the
// break-even point at which Set outperforms linear scan.
func BenchmarkSet_Contains(b *testing.B) {
	for _, n := range []int{5, 20, 100} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			// Arrange — build N string elements; search for the last one
			// (worst case for a linear scan, but O(1) for a hash set).
			elems := make([]string, n)
			for i := range n {
				elems[i] = fmt.Sprintf("element-%d", i)
			}
			target := elems[n-1]
			s := types.NewSet(elems...)

			b.ResetTimer()
			for range b.N {
				_ = s.Contains(target)
			}
		})
	}
}

// BenchmarkSlice_Contains measures O(n) linear membership checks at the same
// sizes used in BenchmarkSet_Contains to provide a direct comparison baseline.
func BenchmarkSlice_Contains(b *testing.B) {
	for _, n := range []int{5, 20, 100} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			// Arrange — same element list as the Set benchmark.
			elems := make([]string, n)
			for i := range n {
				elems[i] = fmt.Sprintf("element-%d", i)
			}
			target := elems[n-1]

			b.ResetTimer()
			for range b.N {
				_ = slices.Contains(elems, target)
			}
		})
	}
}
