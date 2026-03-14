package seed

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestSeedContext_DBTX_PrefersTx(t *testing.T) {
	sc := NewSeedContext(nil)
	assert.Nil(t, sc.Tx())
	// DBTX returns db when no tx is set.
	assert.Nil(t, sc.DBTX())
}

func TestSeedContext_Logger_NopDefault(t *testing.T) {
	sc := NewSeedContext(nil)
	logger := sc.Logger()
	assert.NotNil(t, logger, "should return nop logger, not nil")
}

func TestSeedContext_Options(t *testing.T) {
	logger := zap.NewNop()
	refs := NewReferenceStore()

	sc := NewSeedContext(nil,
		WithLogger(logger),
		WithRefs(refs),
		WithEnv("staging"),
		WithDryRun(true),
	)

	assert.Equal(t, logger, sc.Logger())
	assert.Equal(t, refs, sc.Refs())
	assert.Equal(t, "staging", sc.Env())
	assert.True(t, sc.DryRun())
}

func TestContextHelpers_Seeding(t *testing.T) {
	ctx := context.Background()
	assert.False(t, IsSeeding(ctx))

	ctx = WithSeeding(ctx)
	assert.True(t, IsSeeding(ctx))
}

func TestContextHelpers_EventsDisabled(t *testing.T) {
	ctx := context.Background()
	assert.False(t, EventsDisabled(ctx))

	ctx = WithEventsDisabled(ctx)
	assert.True(t, EventsDisabled(ctx))
}
