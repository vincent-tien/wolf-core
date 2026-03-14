// Package aggregate provides the foundational building block for all aggregate
// roots in the wolf-be domain model.
package aggregate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vincent-tien/wolf-core/event"
)

// stubEvent is a minimal Event implementation used only in tests.
type stubEvent struct {
	id      string
	evtType string
}

func (s *stubEvent) EventID() string       { return s.id }
func (s *stubEvent) EventType() string     { return s.evtType }
func (s *stubEvent) AggregateID() string   { return "agg-1" }
func (s *stubEvent) AggregateType() string { return "Stub" }
func (s *stubEvent) OccurredAt() time.Time { return time.Now() }
func (s *stubEvent) Version() int          { return 1 }
func (s *stubEvent) Payload() any          { return nil }
func (s *stubEvent) GetMetadata() event.Metadata {
	return event.Metadata{}
}

func newStubEvent(id, evtType string) event.Event {
	return &stubEvent{id: id, evtType: evtType}
}

func TestNewBase_IDIsSet(t *testing.T) {
	b := NewBase("order-42", "Order")
	assert.Equal(t, "order-42", b.ID())
}

func TestNewBase_AggregateTypeIsSet(t *testing.T) {
	b := NewBase("order-42", "Order")
	assert.Equal(t, "Order", b.AggregateType())
}

func TestNewBase_VersionStartsAtZero(t *testing.T) {
	b := NewBase("id", "Test")
	assert.Equal(t, 0, b.Version())
}

func TestNewBase_TimestampsAreRecent(t *testing.T) {
	before := time.Now().UTC().Add(-time.Millisecond)
	b := NewBase("id", "Test")
	after := time.Now().UTC().Add(time.Millisecond)

	assert.True(t, b.CreatedAt().After(before))
	assert.True(t, b.CreatedAt().Before(after))
	assert.True(t, b.UpdatedAt().After(before))
	assert.True(t, b.UpdatedAt().Before(after))
}

func TestNewBase_NoEventsByDefault(t *testing.T) {
	b := NewBase("id", "Test")
	assert.False(t, b.HasEvents())
}

func TestIncrementVersion(t *testing.T) {
	tests := []struct {
		name       string
		increments int
		wantVer    int
	}{
		{"one increment", 1, 1},
		{"three increments", 3, 3},
		{"zero increments stays zero", 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBase("id", "Test")
			for range tc.increments {
				b.IncrementVersion()
			}
			assert.Equal(t, tc.wantVer, b.Version())
		})
	}
}

func TestAddEvent_HasEventsReturnsTrue(t *testing.T) {
	b := NewBase("id", "Test")
	b.AddEvent(newStubEvent("e-1", "order.created"))
	assert.True(t, b.HasEvents())
}

func TestAddEvent_MultipleEventsAccumulate(t *testing.T) {
	b := NewBase("id", "Test")
	b.AddEvent(newStubEvent("e-1", "order.created"))
	b.AddEvent(newStubEvent("e-2", "order.confirmed"))
	b.AddEvent(newStubEvent("e-3", "order.shipped"))

	pending := b.ClearEvents()
	assert.Len(t, pending, 3)
}

func TestClearEvents_ReturnsAllPendingEvents(t *testing.T) {
	b := NewBase("id", "Test")
	e1 := newStubEvent("e-1", "a.b")
	e2 := newStubEvent("e-2", "c.d")
	b.AddEvent(e1)
	b.AddEvent(e2)

	pending := b.ClearEvents()

	require.Len(t, pending, 2)
	assert.Equal(t, "e-1", pending[0].EventID())
	assert.Equal(t, "e-2", pending[1].EventID())
}

func TestClearEvents_ResetsHasEventsToFalse(t *testing.T) {
	b := NewBase("id", "Test")
	b.AddEvent(newStubEvent("e-1", "a.b"))
	b.ClearEvents()
	assert.False(t, b.HasEvents())
}

func TestClearEvents_CanAddEventsAfterClear(t *testing.T) {
	b := NewBase("id", "Test")
	b.AddEvent(newStubEvent("e-1", "a.b"))
	b.ClearEvents()

	b.AddEvent(newStubEvent("e-2", "c.d"))
	pending := b.ClearEvents()

	require.Len(t, pending, 1)
	assert.Equal(t, "e-2", pending[0].EventID())
}

func TestSetCreatedAt_RestoresTimestamp(t *testing.T) {
	b := NewBase("id", "Test")
	fixed := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	b.SetCreatedAt(fixed)
	assert.Equal(t, fixed, b.CreatedAt())
}

func TestSetUpdatedAt(t *testing.T) {
	b := NewBase("id", "Test")
	fixed := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	b.SetUpdatedAt(fixed)
	assert.Equal(t, fixed, b.UpdatedAt())
}

func TestAddDomainEvent_SetsAggregateInfo(t *testing.T) {
	b := NewBase("order-42", "Order")
	b.AddDomainEvent("order.placed.v1", nil)

	pending := b.ClearEvents()
	require.Len(t, pending, 1)
	assert.Equal(t, "order-42", pending[0].AggregateID())
	assert.Equal(t, "Order", pending[0].AggregateType())
	assert.Equal(t, "order.placed.v1", pending[0].EventType())
}

func TestAddDomainEvent_WithPayload(t *testing.T) {
	type payload struct{ Name string }
	b := NewBase("p-1", "Product")
	b.AddDomainEvent("product.created.v1", &payload{Name: "Widget"})

	pending := b.ClearEvents()
	require.Len(t, pending, 1)

	got, ok := pending[0].Payload().(*payload)
	require.True(t, ok)
	assert.Equal(t, "Widget", got.Name)
}

func TestAddDomainEvent_WithOptions(t *testing.T) {
	b := NewBase("o-1", "Order")
	b.AddDomainEvent("order.placed.v1", nil,
		event.WithCorrelationID("corr-1"),
		event.WithSource("test"),
	)

	pending := b.ClearEvents()
	require.Len(t, pending, 1)
	meta := pending[0].GetMetadata()
	assert.Equal(t, "corr-1", meta.CorrelationID)
	assert.Equal(t, "test", meta.Source)
}
