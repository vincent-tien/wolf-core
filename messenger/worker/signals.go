// signals.go — OS signal integration for graceful worker shutdown.
package worker

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// RunWithSignals starts the worker and stops on SIGINT/SIGTERM.
func RunWithSignals(ctx context.Context, w *Worker, transports ...string) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	return w.Run(ctx, transports...)
}
