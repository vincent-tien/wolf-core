package types_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/types"
)

func TestNewSet_WithValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []string
		wantLen int
	}{
		{"single value", []string{"a"}, 1},
		{"multiple distinct values", []string{"a", "b", "c"}, 3},
		{"duplicates are collapsed", []string{"x", "x", "x"}, 1},
		{"mixed duplicates", []string{"a", "b", "a", "c", "b"}, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange + Act
			s := types.NewSet(tc.input...)

			// Assert
			assert.Equal(t, tc.wantLen, s.Len())
		})
	}
}

func TestNewSet_Empty(t *testing.T) {
	t.Parallel()

	// Arrange + Act
	s := types.NewSet[string]()

	// Assert
	assert.Equal(t, 0, s.Len())
}

func TestSet_Add(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		initial     []int
		add         int
		wantLen     int
		wantContain bool
	}{
		{"add new element", []int{1, 2}, 3, 3, true},
		{"add duplicate is no-op", []int{1, 2}, 1, 2, true},
		{"add to empty set", []int{}, 5, 1, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			s := types.NewSet(tc.initial...)

			// Act
			s.Add(tc.add)

			// Assert
			assert.Equal(t, tc.wantLen, s.Len())
			assert.Equal(t, tc.wantContain, s.Contains(tc.add))
		})
	}
}

func TestSet_Remove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		initial     []int
		remove      int
		wantLen     int
		wantContain bool
	}{
		{"remove existing element", []int{1, 2, 3}, 2, 2, false},
		{"remove absent element is no-op", []int{1, 2}, 9, 2, false},
		{"remove only element", []int{7}, 7, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			s := types.NewSet(tc.initial...)

			// Act
			s.Remove(tc.remove)

			// Assert
			assert.Equal(t, tc.wantLen, s.Len())
			assert.Equal(t, tc.wantContain, s.Contains(tc.remove))
		})
	}
}

func TestSet_Contains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		initial []string
		query   string
		want    bool
	}{
		{"present element", []string{"admin", "user"}, "admin", true},
		{"absent element", []string{"admin", "user"}, "guest", false},
		{"empty set returns false", []string{}, "admin", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			s := types.NewSet(tc.initial...)

			// Act + Assert
			assert.Equal(t, tc.want, s.Contains(tc.query))
		})
	}
}

func TestSet_ContainsAll(t *testing.T) {
	t.Parallel()

	s := types.NewSet("a", "b", "c")

	tests := []struct {
		name  string
		items []string
		want  bool
	}{
		{"all present", []string{"a", "b", "c"}, true},
		{"subset present", []string{"a", "b"}, true},
		{"single present", []string{"c"}, true},
		{"one absent", []string{"a", "d"}, false},
		{"all absent", []string{"x", "y"}, false},
		{"empty items returns true", []string{}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act + Assert
			assert.Equal(t, tc.want, s.ContainsAll(tc.items...))
		})
	}
}

func TestSet_ContainsAny(t *testing.T) {
	t.Parallel()

	s := types.NewSet("a", "b", "c")

	tests := []struct {
		name  string
		items []string
		want  bool
	}{
		{"one present", []string{"a", "x"}, true},
		{"all present", []string{"a", "b", "c"}, true},
		{"none present", []string{"x", "y"}, false},
		{"empty items returns false", []string{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act + Assert
			assert.Equal(t, tc.want, s.ContainsAny(tc.items...))
		})
	}
}

func TestSet_Len(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []int
		wantLen int
	}{
		{"empty", nil, 0},
		{"single", []int{1}, 1},
		{"multiple", []int{1, 2, 3, 4, 5}, 5},
		{"with duplicates", []int{1, 1, 2, 2}, 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange + Act
			s := types.NewSet(tc.input...)

			// Assert
			assert.Equal(t, tc.wantLen, s.Len())
		})
	}
}

func TestSet_ToSlice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []int
		want  []int
	}{
		{"empty set returns non-nil empty slice", nil, []int{}},
		{"single element", []int{42}, []int{42}},
		{"multiple elements round-trip", []int{1, 2, 3}, []int{1, 2, 3}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			s := types.NewSet(tc.input...)

			// Act
			got := s.ToSlice()

			// Assert — sort both to remove map iteration non-determinism.
			sort.Ints(got)
			sort.Ints(tc.want)
			require.NotNil(t, got)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSet_Intersect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		a     []int
		b     []int
		want  []int
	}{
		{"common elements", []int{1, 2, 3}, []int{2, 3, 4}, []int{2, 3}},
		{"no common elements", []int{1, 2}, []int{3, 4}, []int{}},
		{"identical sets", []int{1, 2, 3}, []int{1, 2, 3}, []int{1, 2, 3}},
		{"one empty set", []int{1, 2, 3}, []int{}, []int{}},
		{"both empty", []int{}, []int{}, []int{}},
		{"subset", []int{1, 2, 3, 4}, []int{2, 4}, []int{2, 4}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			a := types.NewSet(tc.a...)
			b := types.NewSet(tc.b...)

			// Act
			got := a.Intersect(b)
			gotSlice := got.ToSlice()

			// Assert
			sort.Ints(gotSlice)
			sort.Ints(tc.want)
			assert.Equal(t, tc.want, gotSlice)
		})
	}
}

func TestSet_Union(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    []int
		b    []int
		want []int
	}{
		{"disjoint sets", []int{1, 2}, []int{3, 4}, []int{1, 2, 3, 4}},
		{"overlapping sets", []int{1, 2, 3}, []int{2, 3, 4}, []int{1, 2, 3, 4}},
		{"identical sets", []int{1, 2}, []int{1, 2}, []int{1, 2}},
		{"one empty", []int{1, 2, 3}, []int{}, []int{1, 2, 3}},
		{"both empty", []int{}, []int{}, []int{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			a := types.NewSet(tc.a...)
			b := types.NewSet(tc.b...)

			// Act
			got := a.Union(b)
			gotSlice := got.ToSlice()

			// Assert
			sort.Ints(gotSlice)
			sort.Ints(tc.want)
			assert.Equal(t, tc.want, gotSlice)
		})
	}
}

func TestSet_Intersect_DoesNotMutateReceiver(t *testing.T) {
	t.Parallel()

	// Arrange
	a := types.NewSet(1, 2, 3)
	b := types.NewSet(2, 3, 4)
	originalLen := a.Len()

	// Act
	_ = a.Intersect(b)

	// Assert
	assert.Equal(t, originalLen, a.Len())
}

func TestSet_Union_DoesNotMutateReceiver(t *testing.T) {
	t.Parallel()

	// Arrange
	a := types.NewSet(1, 2)
	b := types.NewSet(3, 4)
	originalLen := a.Len()

	// Act
	_ = a.Union(b)

	// Assert
	assert.Equal(t, originalLen, a.Len())
}
