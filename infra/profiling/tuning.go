// tuning.go — Runtime tuning helpers (GOMAXPROCS, GC target, memory limit).
package profiling

import (
	"fmt"
	"runtime/debug"

	"go.uber.org/zap"
)

// TuningConfig holds runtime tuning parameters.
type TuningConfig struct {
	// GCPercent sets the garbage collection target percentage via debug.SetGCPercent.
	// -1 disables GC. 0 means constant collection. Default Go value is 100.
	// A value of 0 in this struct means "do not change" (use current value).
	GCPercent int

	// MemoryLimit sets the soft memory limit via debug.SetMemoryLimit.
	// 0 means "do not change". Specified in bytes.
	// Typical values: 512*1024*1024 (512MB), 1024*1024*1024 (1GB).
	MemoryLimit int64
}

// Apply applies the runtime tuning configuration.
// It returns the previous values so callers can restore them if needed.
// Logs the changes at Info level.
func Apply(cfg TuningConfig, logger *zap.Logger) (prev TuningConfig) {
	if cfg.GCPercent != 0 {
		old := debug.SetGCPercent(cfg.GCPercent)
		prev.GCPercent = old
		logger.Info("runtime tuning: GC percent updated",
			zap.Int("previous", old),
			zap.Int("current", cfg.GCPercent),
		)
	}

	if cfg.MemoryLimit != 0 {
		old := debug.SetMemoryLimit(cfg.MemoryLimit)
		prev.MemoryLimit = old
		logger.Info("runtime tuning: memory limit updated",
			zap.Int64("previous_bytes", old),
			zap.Int64("current_bytes", cfg.MemoryLimit),
		)
	}

	return prev
}

// Validate checks that the TuningConfig values are within acceptable ranges.
func Validate(cfg TuningConfig) error {
	if cfg.GCPercent < -1 {
		return fmt.Errorf("profiling: gc_percent must be >= -1, got %d", cfg.GCPercent)
	}
	if cfg.GCPercent > 2000 {
		return fmt.Errorf("profiling: gc_percent must be <= 2000, got %d", cfg.GCPercent)
	}
	if cfg.MemoryLimit < 0 {
		return fmt.Errorf("profiling: memory_limit must be >= 0, got %d", cfg.MemoryLimit)
	}
	return nil
}
