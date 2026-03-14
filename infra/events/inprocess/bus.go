// Package inprocess provides a goroutine-safe, in-memory implementation of the
// event.Bus interface. Events are fanned out to all registered handlers in
// separate goroutines, allowing handlers to run concurrently without blocking
// the publisher.
package inprocess

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/event"
)

const closeTimeout = 5 * time.Second

// Bus is a goroutine-safe, in-memory event fan-out bus.
// All registered handlers for a given event type are invoked concurrently in
// separate goroutines when an event is published. Handler errors are logged but
// do not affect other handlers or the caller.
//
// Bus is intended for development and test environments. It must not be used
// across process boundaries.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]event.EventHandler
	wg       sync.WaitGroup
	logger   *zap.Logger
}

// NewBus creates and returns an initialised *Bus.
func NewBus(logger *zap.Logger) *Bus {
	return &Bus{
		handlers: make(map[string][]event.EventHandler),
		logger:   logger,
	}
}

// Publish fans out evt to all handlers registered for evt.EventType().
// Each handler is invoked in its own goroutine. Handler errors are logged at
// error level with the event type and handler index; they do not propagate to
// the caller. Publish always returns nil.
func (b *Bus) Publish(ctx context.Context, evt event.Event) error {
	b.mu.RLock()
	// Copy the slice so we can release the read lock before spawning goroutines.
	src := b.handlers[evt.EventType()]
	if len(src) == 0 {
		b.mu.RUnlock()
		return nil
	}
	handlers := make([]event.EventHandler, len(src))
	copy(handlers, src)
	b.mu.RUnlock()

	for i, h := range handlers {
		b.wg.Add(1)
		go func(idx int, handler event.EventHandler) {
			defer b.wg.Done()

			// Detach from parent cancellation but keep values, then strictly apply a deadline.
			bgCtx := context.WithoutCancel(ctx)
			timeoutCtx, cancel := context.WithTimeout(bgCtx, 15*time.Second)
			defer cancel()

			if err := handler(timeoutCtx, evt); err != nil {
				b.logger.Error("inprocess: handler error",
					zap.String("event_type", evt.EventType()),
					zap.String("event_id", evt.EventID()),
					zap.Int("handler_index", idx),
					zap.Error(err),
				)
			}
		}(i, h)
	}

	return nil
}

// Subscribe registers handler for the given eventType. Multiple handlers may
// be registered for the same event type; all are invoked on each Publish call.
// Safe for concurrent use.
func (b *Bus) Subscribe(eventType string, handler event.EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Close waits for all in-flight handler goroutines to finish. It blocks for at
// most closeTimeout (5 s); if that deadline is exceeded it returns an error
// describing the timeout. Close should be called during graceful shutdown.
func (b *Bus) Close() error {
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	timer := time.NewTimer(closeTimeout)
	select {
	case <-done:
		timer.Stop()
		return nil
	case <-timer.C:
		return fmt.Errorf("inprocess: bus close timed out after %s waiting for handlers to finish", closeTimeout)
	}
}
