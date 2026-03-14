// Package bootstrap is the composition root that wires all platform
// dependencies and modules together.
package bootstrap

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/concurrency"
)

// Shutdown performs an ordered graceful teardown of the application.
// Servers (HTTP, gRPC, metrics) are already stopped by Run()'s context-watcher
// before Shutdown is called, so this method handles remaining resources:
//
//	Priority 1 — Modules: drain in-flight work (concurrent within priority)
//	Priority 2 — Event bus + messaging stream: close subscriptions, drain broker
//	Priority 3 — Cache: close Redis connections
//	Priority 4 — DB pools: close write + read pools (concurrent)
//	Priority 5 — Tracer: flush remaining spans to collector
//
// Within the same priority, tasks run concurrently via ShutdownGroup.
// Timeout is config-driven (app.shutdown_timeout, default 30s).
func (a *App) Shutdown() error {
	timeout := a.cfg.App.ShutdownTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var sg concurrency.ShutdownGroup

	// Servers (HTTP, gRPC, metrics) are already stopped by Run()'s
	// context-watcher goroutine before Shutdown is called, so we skip
	// priority 0 and start at priority 1.

	// Priority 1 — modules (concurrent within this priority level).
	for i := len(a.modules) - 1; i >= 0; i-- {
		m := a.modules[i]
		sg.AddFunc(1, fmt.Sprintf("module-%s", m.Name()), func(ctx context.Context) error {
			if err := m.OnStop(ctx); err != nil {
				return fmt.Errorf("bootstrap: module %q OnStop: %w", m.Name(), err)
			}
			a.logger.Info("module stopped", zap.String("module", m.Name()))
			return nil
		})
	}

	// Priority 2 — event bus + messaging stream (concurrent).
	sg.AddFunc(2, "event-bus", func(_ context.Context) error {
		if err := a.eventBus.Close(); err != nil {
			return fmt.Errorf("bootstrap: close event bus: %w", err)
		}
		a.logger.Info("event bus closed")
		return nil
	})

	sg.AddFunc(2, "messaging-stream", func(_ context.Context) error {
		if err := a.stream.Close(); err != nil {
			return fmt.Errorf("bootstrap: close messaging stream: %w", err)
		}
		a.logger.Info("messaging stream closed")
		return nil
	})

	// Priority 3 — cache client.
	sg.AddFunc(3, "cache-client", func(_ context.Context) error {
		if err := a.cacheClient.Close(); err != nil {
			return fmt.Errorf("bootstrap: close cache client: %w", err)
		}
		a.logger.Info("cache client closed")
		return nil
	})

	// Priority 4 — DB pools (write and read run concurrently).
	sg.AddFunc(4, "write-db", func(_ context.Context) error {
		if err := a.writeDB.Close(); err != nil {
			return fmt.Errorf("bootstrap: close write db pool: %w", err)
		}
		a.logger.Info("write db pool closed")
		return nil
	})

	sg.AddFunc(4, "read-db", func(_ context.Context) error {
		if err := a.readDB.Close(); err != nil {
			return fmt.Errorf("bootstrap: close read db pool: %w", err)
		}
		a.logger.Info("read db pool closed")
		return nil
	})

	// Priority 5 — tracer provider (flush remaining spans).
	if a.tracerProvider != nil {
		sg.AddFunc(5, "tracer-provider", func(ctx context.Context) error {
			if err := a.tracerProvider.Shutdown(ctx); err != nil {
				return fmt.Errorf("bootstrap: shutdown tracer provider: %w", err)
			}
			a.logger.Info("tracer provider shut down")
			return nil
		})
	}

	return sg.Shutdown(shutdownCtx)
}
