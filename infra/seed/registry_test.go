package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_RegisterAndSeeders(t *testing.T) {
	r := NewRegistry()

	require.NoError(t, r.Register(&stubSeeder{name: "test.a", groups: []string{"core"}}))
	require.NoError(t, r.Register(&stubSeeder{name: "test.b", groups: []string{"core"}}))

	seeders := r.Seeders()
	require.Len(t, seeders, 2)
	assert.Equal(t, "test.a", seeders[0].Name())
	assert.Equal(t, "test.b", seeders[1].Name())
}

func TestRegistry_DuplicateReturnsError(t *testing.T) {
	r := NewRegistry()

	require.NoError(t, r.Register(&stubSeeder{name: "test.dup"}))
	err := r.Register(&stubSeeder{name: "test.dup"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate seeder")
}

func TestRegistry_SeedersReturnsCopy(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&stubSeeder{name: "test.a"}))

	seeders := r.Seeders()
	seeders[0] = &stubSeeder{name: "modified"}

	assert.Equal(t, "test.a", r.Seeders()[0].Name())
}

func TestRegistry_Names(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&stubSeeder{name: "iam.roles"}))
	require.NoError(t, r.Register(&stubSeeder{name: "iam.admin"}))

	assert.Equal(t, []string{"iam.roles", "iam.admin"}, r.Names())
}

func TestRegistry_EmptyState(t *testing.T) {
	r := NewRegistry()

	assert.Empty(t, r.Seeders())
	assert.Empty(t, r.Names())
}
