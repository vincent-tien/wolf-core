// Package aggregate provides the foundational building block for all aggregate
// roots in the wolf-be domain model. An aggregate root is the consistency
// boundary for a cluster of domain objects, responsible for enforcing invariants
// and collecting domain events.
package aggregate

import (
	"time"

	"github.com/vincent-tien/wolf-core/event"
)

// Base is the embeddable aggregate root base. It tracks identity, aggregate type,
// optimistic concurrency version, domain events, and audit timestamps.
// All fields are unexported; access is via exported getter/setter methods.
//
// Usage:
//
//	type Order struct {
//	    aggregate.Base
//	    // domain fields ...
//	}
type Base struct {
	id            string
	aggregateType string
	version       int
	events        []event.Event
	createdAt     time.Time
	updatedAt     time.Time
}

// NewBase constructs a Base with the given identifier and aggregate type name,
// version 0, a pre-allocated event slice (capacity 4), and both timestamps
// set to the current UTC time.
func NewBase(id, aggregateType string) Base {
	now := time.Now().UTC()
	return Base{
		id:            id,
		aggregateType: aggregateType,
		version:       0,
		events:        make([]event.Event, 0, 4),
		createdAt:     now,
		updatedAt:     now,
	}
}

// ID returns the aggregate's unique string identifier.
func (b *Base) ID() string { return b.id }

// AggregateType returns the type name of this aggregate (e.g., "Product", "Order").
func (b *Base) AggregateType() string { return b.aggregateType }

// Version returns the current optimistic concurrency version of the aggregate.
// The version starts at 0 and is incremented on each state-changing operation.
func (b *Base) Version() int { return b.version }

// IncrementVersion advances the optimistic concurrency version by one.
// Call this inside state-transition methods after applying changes.
func (b *Base) IncrementVersion() { b.version++ }

// SetVersion restores the persisted version during rehydration.
// This must only be called from repository reconstruction — never from use cases.
func (b *Base) SetVersion(v int) { b.version = v }

// CreatedAt returns the UTC time at which the aggregate was created.
func (b *Base) CreatedAt() time.Time { return b.createdAt }

// UpdatedAt returns the UTC time at which the aggregate was last modified.
func (b *Base) UpdatedAt() time.Time { return b.updatedAt }

// SetCreatedAt restores the persisted creation timestamp during rehydration.
// This must only be called from repository reconstruction — never from use cases.
func (b *Base) SetCreatedAt(t time.Time) { b.createdAt = t }

// SetUpdatedAt sets the aggregate's last-modified timestamp.
func (b *Base) SetUpdatedAt(t time.Time) { b.updatedAt = t }

// AddEvent appends a domain event to the aggregate's pending event list.
// Call this inside state-transition methods for every observable state change.
func (b *Base) AddEvent(e event.Event) {
	b.events = append(b.events, e)
}

// AddDomainEvent creates a new domain event with the given type and payload,
// automatically injecting the aggregate's ID, type, and current version,
// and appends it to the pending event list.
func (b *Base) AddDomainEvent(eventType string, payload any, opts ...event.EventOption) {
	allOpts := make([]event.EventOption, 0, len(opts)+1)
	allOpts = append(allOpts, event.WithAggregateInfo(b.id, b.aggregateType))
	allOpts = append(allOpts, opts...)
	evt := event.NewEvent(eventType, payload, allOpts...)
	b.events = append(b.events, evt)
}

// ClearEvents returns all pending domain events and resets the internal slice.
// The returned slice is a defensive copy — subsequent AddEvent calls on the
// aggregate will not mutate the returned events, making it safe for deferred
// or batched publishing (e.g. outbox insertion).
func (b *Base) ClearEvents() []event.Event {
	pending := make([]event.Event, len(b.events))
	copy(pending, b.events)
	b.events = b.events[:0]
	return pending
}

// HasEvents reports whether there are pending domain events awaiting publication.
func (b *Base) HasEvents() bool { return len(b.events) > 0 }
