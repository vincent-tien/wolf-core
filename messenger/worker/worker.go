// Package worker consumes messages from transports and dispatches through the bus.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
	"github.com/vincent-tien/wolf-core/messenger/transport"
)

// Worker consumes messages from transport receivers and dispatches them.
type Worker struct {
	bus       messenger.Bus
	receivers map[string]transport.Receiver
	logger    *slog.Logger
	opts      Options
}

// New creates a worker. Panics on invalid options (programming error at startup).
func New(bus messenger.Bus, receivers map[string]transport.Receiver, opts ...Option) *Worker {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	if o.Concurrency < 1 {
		panic(fmt.Sprintf("worker: concurrency must be >= 1, got %d", o.Concurrency))
	}
	if o.PollInterval <= 0 {
		panic(fmt.Sprintf("worker: poll_interval must be > 0, got %s", o.PollInterval))
	}
	if o.ShutdownTimeout <= 0 {
		panic(fmt.Sprintf("worker: shutdown_timeout must be > 0, got %s", o.ShutdownTimeout))
	}
	return &Worker{
		bus:       bus,
		receivers: receivers,
		logger:    o.Logger,
		opts:      o,
	}
}

// Run starts consuming from the named transports. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context, transportNames ...string) error {
	for _, name := range transportNames {
		if _, ok := w.receivers[name]; !ok {
			return fmt.Errorf("worker: transport %q not registered", name)
		}
	}

	var wg sync.WaitGroup
	for _, name := range transportNames {
		receiver := w.receivers[name]
		for i := range w.opts.Concurrency {
			wg.Add(1)
			go func(tName string, r transport.Receiver, workerID int) {
				defer wg.Done()
				w.consumeLoop(ctx, tName, r, workerID)
			}(name, receiver, i)
		}
	}

	<-ctx.Done()

	// Graceful shutdown: wait for in-flight messages up to ShutdownTimeout.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), w.opts.ShutdownTimeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		w.logger.Info("worker: shutdown complete")
	case <-shutdownCtx.Done():
		w.logger.Warn("worker: shutdown timeout, some messages may not have been processed")
	}

	return nil
}

func (w *Worker) consumeLoop(ctx context.Context, transportName string, receiver transport.Receiver, workerID int) {
	w.logger.Info("worker: consumer started",
		slog.String("transport", transportName),
		slog.Int("worker_id", workerID),
	)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		envelopes, err := receiver.Get(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // context cancelled
			}
			w.logger.Error("worker: get failed",
				slog.String("transport", transportName),
				slog.String("error", err.Error()),
			)
			sleepCtx(ctx, w.opts.PollInterval)
			continue
		}

		if len(envelopes) == 0 {
			sleepCtx(ctx, w.opts.PollInterval)
			continue
		}

		for _, env := range envelopes {
			w.processMessage(ctx, transportName, receiver, env)
		}
	}
}

func (w *Worker) processMessage(ctx context.Context, transportName string, receiver transport.Receiver, env messenger.Envelope) {
	env = env.WithStamp(stamp.ReceivedStamp{
		Transport:  transportName,
		ReceivedAt: time.Now(),
	})

	_, err := w.bus.Dispatch(ctx, env.Message, env.Stamps()...)
	if err != nil {
		w.logger.Error("worker: dispatch failed",
			slog.String("transport", transportName),
			slog.String("message_type", env.MessageTypeName()),
			slog.String("error", err.Error()),
		)
		if rejectErr := receiver.Reject(ctx, env, err); rejectErr != nil {
			w.logger.Error("worker: reject failed",
				slog.String("transport", transportName),
				slog.String("error", rejectErr.Error()),
			)
		}
		return
	}

	if ackErr := receiver.Ack(ctx, env); ackErr != nil {
		w.logger.Error("worker: ack failed",
			slog.String("transport", transportName),
			slog.String("error", ackErr.Error()),
		)
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
