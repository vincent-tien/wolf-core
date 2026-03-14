// Package event defines the core domain event contracts for the wolf-be platform.
package event

import (
	"time"

	"github.com/google/uuid"
)

// Event represents a domain event emitted by an aggregate root.
// All domain events must implement this interface to participate in the
// event bus and outbox publishing pipeline.
type Event interface {
	// EventID returns the unique identifier for this event instance.
	EventID() string
	// EventType returns the fully-qualified event type name (e.g., "product.created.v1").
	EventType() string
	// AggregateID returns the ID of the aggregate that emitted this event.
	AggregateID() string
	// AggregateType returns the type name of the aggregate (e.g., "Order").
	AggregateType() string
	// OccurredAt returns the UTC timestamp when the event occurred.
	OccurredAt() time.Time
	// Version returns the schema version of the event payload.
	Version() int
	// Payload returns the typed event payload, or nil if not set.
	Payload() any
	// GetMetadata returns the cross-cutting metadata for tracing and correlation.
	GetMetadata() Metadata
}

// Metadata carries cross-cutting context for event tracing and correlation.
// It is propagated alongside every event to enable distributed tracing.
type Metadata struct {
	// TraceID is the distributed trace identifier for this request chain.
	TraceID string `json:"trace_id,omitempty"`
	// CorrelationID groups related events across service boundaries.
	CorrelationID string `json:"correlation_id,omitempty"`
	// CausationID identifies the event or command that caused this event.
	CausationID string `json:"causation_id,omitempty"`
	// Source identifies the service or component that emitted the event.
	Source string `json:"source,omitempty"`
}

// IsZero reports whether all metadata fields are empty.
func (m Metadata) IsZero() bool {
	return m.TraceID == "" && m.CorrelationID == "" && m.CausationID == "" && m.Source == ""
}

// ToMap converts Metadata to a string map for serialization.
// Returns nil when all fields are empty to avoid an unnecessary allocation.
func (m Metadata) ToMap() map[string]string {
	n := 0
	if m.TraceID != "" {
		n++
	}
	if m.CorrelationID != "" {
		n++
	}
	if m.CausationID != "" {
		n++
	}
	if m.Source != "" {
		n++
	}
	if n == 0 {
		return nil
	}

	result := make(map[string]string, n)
	if m.TraceID != "" {
		result["trace_id"] = m.TraceID
	}
	if m.CorrelationID != "" {
		result["correlation_id"] = m.CorrelationID
	}
	if m.CausationID != "" {
		result["causation_id"] = m.CausationID
	}
	if m.Source != "" {
		result["source"] = m.Source
	}
	return result
}

// MetadataFromMap constructs Metadata from a string map.
func MetadataFromMap(m map[string]string) Metadata {
	return Metadata{
		TraceID:       m["trace_id"],
		CorrelationID: m["correlation_id"],
		CausationID:   m["causation_id"],
		Source:        m["source"],
	}
}

// EventOption configures an event during construction via NewEvent.
// The interface method is unexported to prevent external implementations;
// use EventOptionFunc to create custom options.
type EventOption interface {
	configureEvent(e *baseEvent)
}

// EventOptionFunc is an adapter for using plain functions as EventOption.
type EventOptionFunc func(e *baseEvent)

func (f EventOptionFunc) configureEvent(e *baseEvent) { f(e) }

// baseEvent is the internal implementation of Event.
// Create instances via NewEvent().
type baseEvent struct {
	id            string
	eventType     string
	aggID         string
	aggType       string
	occurred      time.Time
	schemaVersion int
	payload       any
	meta          Metadata
}

func (e *baseEvent) EventID() string       { return e.id }
func (e *baseEvent) EventType() string     { return e.eventType }
func (e *baseEvent) AggregateID() string   { return e.aggID }
func (e *baseEvent) AggregateType() string { return e.aggType }
func (e *baseEvent) OccurredAt() time.Time { return e.occurred }
func (e *baseEvent) Version() int          { return e.schemaVersion }
func (e *baseEvent) Payload() any          { return e.payload }
func (e *baseEvent) GetMetadata() Metadata { return e.meta }

// NewEvent creates a new domain event with the given type and payload.
// Options configure metadata, aggregate info, and schema version.
func NewEvent(eventType string, payload any, opts ...EventOption) Event {
	e := &baseEvent{
		id:            uuid.New().String(),
		eventType:     eventType,
		payload:       payload,
		occurred:      time.Now().UTC(),
		schemaVersion: 1,
	}
	for _, opt := range opts {
		opt.configureEvent(e)
	}
	return e
}

// --- Built-in EventOption constructors ---

// WithCorrelationID sets the correlation ID for grouping related events.
func WithCorrelationID(id string) EventOption {
	return EventOptionFunc(func(e *baseEvent) { e.meta.CorrelationID = id })
}

// WithCausationID sets the causation ID identifying the cause of this event.
func WithCausationID(id string) EventOption {
	return EventOptionFunc(func(e *baseEvent) { e.meta.CausationID = id })
}

// WithTraceID sets the distributed trace identifier.
func WithTraceID(id string) EventOption {
	return EventOptionFunc(func(e *baseEvent) { e.meta.TraceID = id })
}

// WithSource sets the originating service/component identifier.
func WithSource(source string) EventOption {
	return EventOptionFunc(func(e *baseEvent) { e.meta.Source = source })
}

// WithVersion sets the schema version of the event payload.
func WithVersion(v int) EventOption {
	return EventOptionFunc(func(e *baseEvent) { e.schemaVersion = v })
}

// WithAggregateInfo sets the aggregate ID and type on the event.
func WithAggregateInfo(id, aggType string) EventOption {
	return EventOptionFunc(func(e *baseEvent) {
		e.aggID = id
		e.aggType = aggType
	})
}

// WithMetadata sets the full metadata on the event.
func WithMetadata(meta Metadata) EventOption {
	return EventOptionFunc(func(e *baseEvent) { e.meta = meta })
}

// WithID overrides the auto-generated UUID. Used to reconstruct events from
// external sources (e.g., message broker headers) preserving original identity.
func WithID(id string) EventOption {
	return EventOptionFunc(func(e *baseEvent) {
		if id != "" {
			e.id = id
		}
	})
}

// WithOccurredAt overrides the auto-generated timestamp. Used to reconstruct
// events preserving original occurrence time from serialized headers.
func WithOccurredAt(t time.Time) EventOption {
	return EventOptionFunc(func(e *baseEvent) {
		if !t.IsZero() {
			e.occurred = t.UTC()
		}
	})
}
