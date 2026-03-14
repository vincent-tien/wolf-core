package types_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/types"
)

// ── Map ──────────────────────────────────────────────────────────────────────

func TestMap_NilInputReturnsNonNilEmptySlice(t *testing.T) {
	t.Parallel()

	// Arrange + Act
	got := types.Map(nil, func(v int) int { return v * 2 })

	// Assert — nil would marshal as JSON null; empty slice marshals as [].
	require.NotNil(t, got)
	assert.Empty(t, got)
}

func TestMap_EmptyInputReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	got := types.Map([]int{}, func(v int) int { return v })

	require.NotNil(t, got)
	assert.Empty(t, got)
}

func TestMap_TransformsElements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []int
		fn    func(int) int
		want  []int
	}{
		{"double", []int{1, 2, 3}, func(v int) int { return v * 2 }, []int{2, 4, 6}},
		{"negate", []int{1, -2, 3}, func(v int) int { return -v }, []int{-1, 2, -3}},
		{"identity", []int{5, 10}, func(v int) int { return v }, []int{5, 10}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := types.Map(tc.input, tc.fn)

			// Assert
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMap_TypeConversion(t *testing.T) {
	t.Parallel()

	// Arrange
	input := []int{1, 2, 3}

	// Act — convert int → string representation length
	got := types.Map(input, func(v int) bool { return v > 1 })

	// Assert
	assert.Equal(t, []bool{false, true, true}, got)
}

// ── Filter ────────────────────────────────────────────────────────────────────

func TestFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []int
		fn    func(int) bool
		want  []int
	}{
		{"keep evens", []int{1, 2, 3, 4, 5}, func(v int) bool { return v%2 == 0 }, []int{2, 4}},
		{"keep all", []int{2, 4, 6}, func(v int) bool { return v%2 == 0 }, []int{2, 4, 6}},
		{"keep none", []int{1, 3, 5}, func(v int) bool { return v%2 == 0 }, []int{}},
		{"empty input", []int{}, func(v int) bool { return true }, []int{}},
		{"nil input", nil, func(v int) bool { return true }, []int{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := types.Filter(tc.input, tc.fn)

			// Assert
			require.NotNil(t, got)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ── Reduce ────────────────────────────────────────────────────────────────────

func TestReduce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []int
		initial int
		fn      func(int, int) int
		want    int
	}{
		{
			name:    "sum",
			input:   []int{1, 2, 3, 4},
			initial: 0,
			fn:      func(acc, v int) int { return acc + v },
			want:    10,
		},
		{
			name:    "product",
			input:   []int{2, 3, 4},
			initial: 1,
			fn:      func(acc, v int) int { return acc * v },
			want:    24,
		},
		{
			name:    "empty input returns initial",
			input:   []int{},
			initial: 42,
			fn:      func(acc, v int) int { return acc + v },
			want:    42,
		},
		{
			name:    "nil input returns initial",
			input:   nil,
			initial: 7,
			fn:      func(acc, v int) int { return acc + v },
			want:    7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := types.Reduce(tc.input, tc.initial, tc.fn)

			// Assert
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReduce_TypeChange(t *testing.T) {
	t.Parallel()

	// Arrange — sum lengths of strings
	input := []string{"go", "lang", "wolf"}

	// Act
	got := types.Reduce(input, 0, func(acc int, s string) int { return acc + len(s) })

	// Assert
	assert.Equal(t, 10, got)
}

// ── GroupBy ───────────────────────────────────────────────────────────────────

func TestGroupBy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []int
		fn    func(int) string
		want  map[string][]int
	}{
		{
			name:  "group by parity",
			input: []int{1, 2, 3, 4, 5},
			fn: func(v int) string {
				if v%2 == 0 {
					return "even"
				}
				return "odd"
			},
			want: map[string][]int{"odd": {1, 3, 5}, "even": {2, 4}},
		},
		{
			name:  "all same key",
			input: []int{1, 2, 3},
			fn:    func(_ int) string { return "all" },
			want:  map[string][]int{"all": {1, 2, 3}},
		},
		{
			name:  "empty input returns empty map",
			input: []int{},
			fn:    func(v int) string { return "k" },
			want:  map[string][]int{},
		},
		{
			name:  "nil input returns empty map",
			input: nil,
			fn:    func(v int) string { return "k" },
			want:  map[string][]int{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := types.GroupBy(tc.input, tc.fn)

			// Assert — sort each bucket for deterministic comparison.
			for k := range got {
				sort.Ints(got[k])
			}
			for k := range tc.want {
				sort.Ints(tc.want[k])
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// ── Contains ─────────────────────────────────────────────────────────────────

func TestContains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		items []string
		v     string
		want  bool
	}{
		{"present element", []string{"a", "b", "c"}, "b", true},
		{"absent element", []string{"a", "b", "c"}, "z", false},
		{"empty slice returns false", []string{}, "a", false},
		{"nil slice returns false", nil, "a", false},
		{"single match", []string{"only"}, "only", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act + Assert
			assert.Equal(t, tc.want, types.Contains(tc.items, tc.v))
		})
	}
}

// ── Unique ────────────────────────────────────────────────────────────────────

func TestUnique(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []int
		want  []int
	}{
		{"no duplicates returns same order", []int{1, 2, 3}, []int{1, 2, 3}},
		{"duplicates removed preserving first occurrence", []int{1, 2, 1, 3, 2}, []int{1, 2, 3}},
		{"all duplicates", []int{5, 5, 5}, []int{5}},
		{"empty input", []int{}, []int{}},
		{"nil input", nil, []int{}},
		{"single element", []int{42}, []int{42}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := types.Unique(tc.input)

			// Assert
			require.NotNil(t, got)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUnique_PreservesInsertionOrder(t *testing.T) {
	t.Parallel()

	// Arrange
	input := []string{"c", "a", "b", "a", "c", "d"}

	// Act
	got := types.Unique(input)

	// Assert — order must reflect first occurrence, not sort order.
	assert.Equal(t, []string{"c", "a", "b", "d"}, got)
}
