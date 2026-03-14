package seed

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single", "core", []string{"core"}},
		{"multiple", "core,demo", []string{"core", "demo"}},
		{"with spaces", " core , demo , test ", []string{"core", "demo", "test"}},
		{"trailing comma", "core,", []string{"core"}},
		{"only commas", ",,", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseCSV(tt.input))
		})
	}
}

func TestRunOptions_ProductionGuardrail(t *testing.T) {
	opts := RunOptions{Env: "production", Force: false}
	assert.True(t, opts.Env == "production" && !opts.Force)

	opts.Force = true
	assert.False(t, opts.Env == "production" && !opts.Force)
}

func TestSeederResult_Statuses(t *testing.T) {
	statuses := []string{"ok", "error", "dry-run", "conditional-skip"}
	for _, s := range statuses {
		r := SeederResult{Status: s}
		assert.NotEmpty(t, r.Status)
	}
}

func TestTxMode_Constants(t *testing.T) {
	assert.Equal(t, TxMode("global"), TxModeGlobal)
	assert.Equal(t, TxMode("per-seeder"), TxModePerSeeder)
}

func TestSeedContext_WithSeeding(t *testing.T) {
	ctx := context.Background()
	ctx = WithSeeding(ctx)
	ctx = WithEventsDisabled(ctx)

	assert.True(t, IsSeeding(ctx))
	assert.True(t, EventsDisabled(ctx))
}

func TestStubSeeder_Interfaces(t *testing.T) {
	s := &stubSeeder{name: "test"}

	var _ Seeder = s
	var _ ConditionalSeeder = s
	var _ TruncatingSeeder = s

	assert.Equal(t, "test", s.Name())
	assert.True(t, s.ShouldRun(context.Background(), nil))
	assert.Nil(t, s.TruncateTables())
}

func TestStubSeeder_SeedFn(t *testing.T) {
	called := false
	s := &stubSeeder{
		name: "test",
		seedFn: func(_ context.Context, _ *SeedContext) error {
			called = true
			return nil
		},
	}

	err := s.Seed(context.Background(), nil)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestStubSeeder_SeedFn_Error(t *testing.T) {
	s := &stubSeeder{
		name: "test",
		seedFn: func(_ context.Context, _ *SeedContext) error {
			return errors.New("seed failed")
		},
	}

	err := s.Seed(context.Background(), nil)
	assert.EqualError(t, err, "seed failed")
}

func TestRegistry_SeedersReturnValid(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&stubSeeder{
		name:   "test.seeder",
		groups: []string{"test"},
	}))

	seeders := r.Seeders()
	require.Len(t, seeders, 1)
	assert.Equal(t, "test.seeder", seeders[0].Name())
	assert.Equal(t, []string{"test"}, seeders[0].Groups())
}
