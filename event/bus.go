// bus.go — In-process event bus contracts and default implementation.
//
// The Bus interface decouples event publishers (aggregates, use cases) from
// subscribers (projections, side-effect handlers). Modules register subscribers
// during bootstrap via module.RegisterSubscribers(bus); publishers call
// bus.Publish() to fan out events synchronously.
//
// For durable cross-service messaging, use the messaging.Stream abstraction
// instead. The in-process Bus is for within-process event dispatch (e.g.,
// updating read models, sending notifications within the same service).
//
// The InMemoryBus is the only provided Bus implementation, suitable for
// local development and within-process dispatch. For durable cross-service
// messaging, use messaging.Stream (backed by NATS JetStream in production)
// via platform/events.NewStream().
package event

import (
	"context"
	"sync"
)

// EventHandler is a function that processes a single domain event.
// Implementations must be idempotent where possible. A non-nil error
// causes the bus to stop processing further handlers for that event.
type EventHandler func(ctx context.Context, evt Event) error

// Publisher is the write side of the event bus contract.
// It publishes a domain event to all registered subscribers.
type Publisher interface {
	// Publish dispatches the event to all registered handlers for its type.
	// Returns the first handler error encountered, if any.
	Publish(ctx context.Context, evt Event) error
}

// Subscriber is the read side of the event bus contract.
// It registers handlers to be called when a specific event type is published.
type Subscriber interface {
	// Subscribe registers a handler for the given event type.
	// Multiple handlers may be registered for the same event type.
	Subscribe(eventType string, handler EventHandler)
}

// Bus is the full event bus interface combining publish, subscribe, and lifecycle.
// Implementations are responsible for handler fanout and error propagation.
type Bus interface {
	Publisher
	Subscriber
	// Close releases any resources held by the bus implementation.
	Close() error
}

// InMemoryBus is a synchronous in-process implementation of Bus.
// Suitable for unit tests and local development; not for production use
// across service boundaries.
type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]EventHandler
}

// NewInMemoryBus creates a new InMemoryBus with an initialised handler map.
func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		handlers: make(map[string][]EventHandler),
	}
}

// Publish dispatches the event to all handlers registered for its type.
// Handlers are called synchronously in registration order. The first error
// aborts the remaining handlers and is returned to the caller.
func (b *InMemoryBus) Publish(ctx context.Context, evt Event) error {
	b.mu.RLock()
	handlers := b.handlers[evt.EventType()]
	b.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

// Subscribe registers a handler for the given event type.
// Safe for concurrent use.
func (b *InMemoryBus) Subscribe(eventType string, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Close is a no-op for the in-memory bus. It exists to satisfy the Bus interface.
func (b *InMemoryBus) Close() error { return nil }
