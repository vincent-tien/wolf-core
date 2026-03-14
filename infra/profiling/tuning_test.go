package profiling_test

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/vincent-tien/wolf-core/infra/profiling"
)

// Apply tests modify global runtime state and MUST NOT run in parallel.

func TestApply_SetsGCPercent(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)

	// Capture the baseline before the test mutates anything.
	baseline := debug.SetGCPercent(100)
	debug.SetGCPercent(baseline) // restore immediately after read

	cfg := profiling.TuningConfig{GCPercent: 200}

	// Act
	prev := profiling.Apply(cfg, logger)

	// Restore: reads the currently set value, then sets it back to prev.
	current := debug.SetGCPercent(prev.GCPercent)

	// Assert — the value in effect after Apply must be 200.
	assert.Equal(t, 200, current)
}

func TestApply_SetsMemoryLimit(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)

	// Capture the baseline.
	baseline := debug.SetMemoryLimit(0)
	debug.SetMemoryLimit(baseline)

	const limit int64 = 512 * 1024 * 1024 // 512 MB

	cfg := profiling.TuningConfig{MemoryLimit: limit}

	// Act
	prev := profiling.Apply(cfg, logger)

	// Restore and capture the value that was in effect.
	current := debug.SetMemoryLimit(prev.MemoryLimit)

	// Assert
	assert.Equal(t, limit, current)
}

func TestApply_ZeroValuesNoChange(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)

	beforeGC := debug.SetGCPercent(100)
	debug.SetGCPercent(beforeGC)

	beforeMem := debug.SetMemoryLimit(0)
	debug.SetMemoryLimit(beforeMem)

	cfg := profiling.TuningConfig{} // all zeros — do nothing

	// Act
	prev := profiling.Apply(cfg, logger)

	// Assert — returned previous must be zero (no fields were changed).
	assert.Equal(t, 0, prev.GCPercent)
	assert.Equal(t, int64(0), prev.MemoryLimit)

	// Verify runtime state is unchanged.
	afterGC := debug.SetGCPercent(beforeGC)
	assert.Equal(t, beforeGC, afterGC, "GC percent must not have changed")

	afterMem := debug.SetMemoryLimit(beforeMem)
	assert.Equal(t, beforeMem, afterMem, "memory limit must not have changed")
}

func TestApply_RestoresPrevious(t *testing.T) {
	// Arrange
	logger := zaptest.NewLogger(t)

	// Record the true original state.
	originalGC := debug.SetGCPercent(100)
	debug.SetGCPercent(originalGC)

	originalMem := debug.SetMemoryLimit(0)
	debug.SetMemoryLimit(originalMem)

	first := profiling.TuningConfig{GCPercent: 300, MemoryLimit: 256 * 1024 * 1024}

	// Act — apply, then restore.
	prev := profiling.Apply(first, logger)
	profiling.Apply(profiling.TuningConfig{
		GCPercent:   prev.GCPercent,
		MemoryLimit: prev.MemoryLimit,
	}, logger)

	// Assert — runtime state is back to original.
	restoredGC := debug.SetGCPercent(originalGC)
	assert.Equal(t, originalGC, restoredGC, "GC percent must be restored to original")

	restoredMem := debug.SetMemoryLimit(originalMem)
	assert.Equal(t, originalMem, restoredMem, "memory limit must be restored to original")
}

// Validate tests are stateless and CAN run in parallel.

func TestValidate_Valid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  profiling.TuningConfig
	}{
		{"zero values", profiling.TuningConfig{}},
		{"gc disable", profiling.TuningConfig{GCPercent: -1}},
		{"gc default", profiling.TuningConfig{GCPercent: 100}},
		{"gc max", profiling.TuningConfig{GCPercent: 2000}},
		{"memory limit 512MB", profiling.TuningConfig{MemoryLimit: 512 * 1024 * 1024}},
		{"both fields set", profiling.TuningConfig{GCPercent: 50, MemoryLimit: 1024 * 1024 * 1024}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			err := profiling.Validate(tc.cfg)

			// Assert
			require.NoError(t, err)
		})
	}
}

func TestValidate_GCPercentTooLow(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := profiling.TuningConfig{GCPercent: -2}

	// Act
	err := profiling.Validate(cfg)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gc_percent must be >= -1")
	assert.Contains(t, err.Error(), "-2")
}

func TestValidate_GCPercentTooHigh(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := profiling.TuningConfig{GCPercent: 2001}

	// Act
	err := profiling.Validate(cfg)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gc_percent must be <= 2000")
	assert.Contains(t, err.Error(), "2001")
}

func TestValidate_NegativeMemoryLimit(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := profiling.TuningConfig{MemoryLimit: -1}

	// Act
	err := profiling.Validate(cfg)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory_limit must be >= 0")
	assert.Contains(t, err.Error(), "-1")
}
