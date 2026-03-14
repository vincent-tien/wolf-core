// dispatcher.go — Compile-time type-safe event dispatch with typed handlers.
package event

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

// TypedHandler processes a domain event with a compile-time type-safe payload.
// T is the concrete payload type (e.g., ProductCreatedPayload).
// The handler receives both the full Event envelope and the extracted payload.
type TypedHandler[T any] func(ctx context.Context, evt Event, payload T) error

// EventDispatcher provides compile-time type-safe event dispatch for a specific
// payload type T. Multiple handlers can be registered; they execute in
// registration order. A handler error stops further processing.
//
// EventDispatcher is safe for concurrent use. Register snapshots the handler
// slice on write (atomic pointer swap); Dispatch reads the snapshot with zero
// allocations on the hot path.
type EventDispatcher[T any] struct {
	mu       sync.Mutex
	snapshot atomic.Pointer[[]TypedHandler[T]]
}

// NewEventDispatcher creates a new EventDispatcher for payload type T.
func NewEventDispatcher[T any]() *EventDispatcher[T] {
	d := &EventDispatcher[T]{}
	empty := make([]TypedHandler[T], 0)
	d.snapshot.Store(&empty)
	return d
}

// Register adds a typed handler to the dispatcher.
// A new snapshot is created and atomically published so that concurrent
// Dispatch calls see the update without allocation on the read path.
func (d *EventDispatcher[T]) Register(handler TypedHandler[T]) {
	d.mu.Lock()
	defer d.mu.Unlock()
	old := *d.snapshot.Load()
	updated := make([]TypedHandler[T], len(old)+1)
	copy(updated, old)
	updated[len(old)] = handler
	d.snapshot.Store(&updated)
}

// Dispatch extracts the payload from evt, asserts it to T, and calls all
// registered handlers in order. It also handles *T payloads as returned by
// TypeRegistry.Deserialize, transparently dereferencing the pointer.
//
// Returns an error if the payload type assertion fails or any handler returns
// an error. On handler error the remaining handlers are skipped.
func (d *EventDispatcher[T]) Dispatch(ctx context.Context, evt Event) error {
	handlers := *d.snapshot.Load()

	payload, ok := evt.Payload().(T)
	if !ok {
		// TypeRegistry.Deserialize returns a pointer; try *T → T dereference.
		payloadPtr, okPtr := evt.Payload().(*T)
		if !okPtr {
			return fmt.Errorf("event: dispatcher expected payload type %T, got %T",
				*new(T), evt.Payload())
		}
		payload = *payloadPtr
	}

	for _, h := range handlers {
		if err := h(ctx, evt, payload); err != nil {
			return err
		}
	}
	return nil
}

// AsEventHandler returns an untyped EventHandler that delegates to this
// dispatcher. Use this to bridge a typed EventDispatcher into the untyped
// Bus.Subscribe system without losing type safety at the handler level.
func (d *EventDispatcher[T]) AsEventHandler() EventHandler {
	return func(ctx context.Context, evt Event) error {
		return d.Dispatch(ctx, evt)
	}
}

// EventHandlerWithLogging wraps an EventHandler with structured logging.
// It logs event metadata on entry and logs duration and any error on exit.
// name identifies the handler in log output (e.g., "SendWelcomeEmail").
func EventHandlerWithLogging(handler EventHandler, logger *zap.Logger, name string) EventHandler {
	return func(ctx context.Context, evt Event) error {
		logger.Debug("handling event",
			zap.String("handler", name),
			zap.String("event_type", evt.EventType()),
			zap.String("event_id", evt.EventID()),
			zap.String("aggregate_id", evt.AggregateID()),
		)

		err := handler(ctx, evt)
		if err != nil {
			logger.Error("event handler failed",
				zap.String("handler", name),
				zap.String("event_type", evt.EventType()),
				zap.String("event_id", evt.EventID()),
				zap.Error(err),
			)
		}
		return err
	}
}
