package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopologicalSort_NoDeps(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "a"},
		&stubSeeder{name: "b"},
		&stubSeeder{name: "c"},
	}

	sorted, err := topologicalSort(seeders)
	require.NoError(t, err)
	require.Len(t, sorted, 3)
	assert.Equal(t, "a", sorted[0].Name())
	assert.Equal(t, "b", sorted[1].Name())
	assert.Equal(t, "c", sorted[2].Name())
}

func TestTopologicalSort_LinearChain(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "c", deps: []string{"b"}},
		&stubSeeder{name: "a"},
		&stubSeeder{name: "b", deps: []string{"a"}},
	}

	sorted, err := topologicalSort(seeders)
	require.NoError(t, err)
	require.Len(t, sorted, 3)

	idx := make(map[string]int)
	for i, s := range sorted {
		idx[s.Name()] = i
	}
	assert.Less(t, idx["a"], idx["b"])
	assert.Less(t, idx["b"], idx["c"])
}

func TestTopologicalSort_Diamond(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "d", deps: []string{"b", "c"}},
		&stubSeeder{name: "b", deps: []string{"a"}},
		&stubSeeder{name: "c", deps: []string{"a"}},
		&stubSeeder{name: "a"},
	}

	sorted, err := topologicalSort(seeders)
	require.NoError(t, err)
	require.Len(t, sorted, 4)

	idx := make(map[string]int)
	for i, s := range sorted {
		idx[s.Name()] = i
	}
	assert.Less(t, idx["a"], idx["b"])
	assert.Less(t, idx["a"], idx["c"])
	assert.Less(t, idx["b"], idx["d"])
	assert.Less(t, idx["c"], idx["d"])
}

func TestTopologicalSort_CycleDetected(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "a", deps: []string{"b"}},
		&stubSeeder{name: "b", deps: []string{"a"}},
	}

	_, err := topologicalSort(seeders)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestTopologicalSort_MissingDep(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "a", deps: []string{"nonexistent"}},
	}

	_, err := topologicalSort(seeders)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unregistered")
}

func TestTopologicalSort_Empty(t *testing.T) {
	sorted, err := topologicalSort(nil)
	require.NoError(t, err)
	assert.Nil(t, sorted)
}

func TestTopologicalSort_SingleNode(t *testing.T) {
	seeders := []Seeder{&stubSeeder{name: "only"}}

	sorted, err := topologicalSort(seeders)
	require.NoError(t, err)
	require.Len(t, sorted, 1)
	assert.Equal(t, "only", sorted[0].Name())
}

func TestTopologicalSort_MultipleDeps(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "leaf", deps: []string{"dep1", "dep2", "dep3"}},
		&stubSeeder{name: "dep1"},
		&stubSeeder{name: "dep2"},
		&stubSeeder{name: "dep3"},
	}

	sorted, err := topologicalSort(seeders)
	require.NoError(t, err)
	require.Len(t, sorted, 4)
	assert.Equal(t, "leaf", sorted[3].Name())
}
