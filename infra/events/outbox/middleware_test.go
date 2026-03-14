package outbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/events/outbox"
	"github.com/vincent-tien/wolf-core/messaging"
)

// --- fakes ---

// fakeInserter is a test double for the outbox Store that records the last
// inserted entry so tests can assert on the stored values.
// It implements outbox.EntryInserter.
type fakeInserter struct {
	inserted []outbox.OutboxEntry
	err      error
}

var _ outbox.EntryInserter = (*fakeInserter)(nil)

func (f *fakeInserter) InsertEntry(_ context.Context, entry outbox.OutboxEntry) error {
	if f.err != nil {
		return f.err
	}
	f.inserted = append(f.inserted, entry)
	return nil
}

// --- compile-time interface check ---

var _ messaging.Publisher = (*outbox.OutboxMiddleware)(nil)

// --- tests ---

func TestOutboxMiddleware_Publish_WritesToStore(t *testing.T) {
	// Arrange
	fake := &fakeInserter{}
	mw := outbox.NewOutboxMiddlewareWithInserter(fake)

	msg := messaging.RawMessage{
		ID:      "evt-001",
		Name:    "order.created.v1",
		Subject: "orders",
		Data:    []byte(`{"order_id":"o-1"}`),
		Headers: map[string]string{
			"aggregate_type": "Order",
			"aggregate_id":   "o-1",
			"event_type":     "order.created.v1",
			"trace_id":       "trace-abc",
		},
	}

	// Act
	err := mw.Publish(context.Background(), "orders", msg)

	// Assert
	require.NoError(t, err)
	require.Len(t, fake.inserted, 1)
	got := fake.inserted[0]
	assert.Equal(t, "evt-001", got.ID)
	assert.Equal(t, "Order", got.AggregateType)
	assert.Equal(t, "o-1", got.AggregateID)
	assert.Equal(t, "order.created.v1", got.EventType)
	assert.Equal(t, msg.Data, got.Payload)
	assert.Equal(t, "trace-abc", got.TraceID)
}

func TestOutboxMiddleware_Publish_EventTypeFallsBackToMsgName(t *testing.T) {
	// Arrange — no "event_type" header; Name should be used instead.
	fake := &fakeInserter{}
	mw := outbox.NewOutboxMiddlewareWithInserter(fake)

	msg := messaging.RawMessage{
		ID:      "evt-002",
		Name:    "product.updated.v1",
		Subject: "products",
		Data:    []byte(`{}`),
		Headers: map[string]string{},
	}

	// Act
	err := mw.Publish(context.Background(), "products", msg)

	// Assert
	require.NoError(t, err)
	require.Len(t, fake.inserted, 1)
	assert.Equal(t, "product.updated.v1", fake.inserted[0].EventType)
}

func TestOutboxMiddleware_Publish_EventTypeFallsBackToSubject(t *testing.T) {
	// Arrange — neither "event_type" header nor Name; subject should be used.
	fake := &fakeInserter{}
	mw := outbox.NewOutboxMiddlewareWithInserter(fake)

	msg := messaging.RawMessage{
		ID:      "evt-003",
		Subject: "inventory",
		Data:    []byte(`{}`),
		Headers: map[string]string{},
	}

	// Act
	err := mw.Publish(context.Background(), "inventory", msg)

	// Assert
	require.NoError(t, err)
	require.Len(t, fake.inserted, 1)
	assert.Equal(t, "inventory", fake.inserted[0].EventType)
}

func TestOutboxMiddleware_Publish_EmptyIDReturnsError(t *testing.T) {
	// Arrange — empty message ID must be rejected.
	fake := &fakeInserter{}
	mw := outbox.NewOutboxMiddlewareWithInserter(fake)

	msg := messaging.RawMessage{
		ID:      "", // intentionally empty
		Subject: "orders",
		Data:    []byte(`{}`),
		Headers: map[string]string{"event_type": "order.created.v1"},
	}

	// Act
	err := mw.Publish(context.Background(), "orders", msg)

	// Assert
	require.Error(t, err)
	assert.Empty(t, fake.inserted)
}

func TestOutboxMiddleware_Publish_StoreErrorPropagated(t *testing.T) {
	// Arrange — store returns an error; middleware must propagate it.
	storeErr := errors.New("db: connection refused")
	fake := &fakeInserter{err: storeErr}
	mw := outbox.NewOutboxMiddlewareWithInserter(fake)

	msg := messaging.RawMessage{
		ID:      "evt-004",
		Subject: "orders",
		Data:    []byte(`{}`),
		Headers: map[string]string{"event_type": "order.created.v1"},
	}

	// Act
	err := mw.Publish(context.Background(), "orders", msg)

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, storeErr)
}

func TestOutboxMiddleware_Publish_HeadersPreservedInEntry(t *testing.T) {
	// Arrange — verify all supported headers are mapped to entry fields.
	fake := &fakeInserter{}
	mw := outbox.NewOutboxMiddlewareWithInserter(fake)

	msg := messaging.RawMessage{
		ID:      "evt-005",
		Subject: "items",
		Data:    []byte(`{"sku":"ABC"}`),
		Headers: map[string]string{
			"aggregate_type": "Item",
			"aggregate_id":   "item-99",
			"event_type":     "item.stocked.v1",
			"trace_id":       "trace-xyz",
		},
	}

	// Act
	require.NoError(t, mw.Publish(context.Background(), "items", msg))

	// Assert
	got := fake.inserted[0]
	assert.Equal(t, "Item", got.AggregateType)
	assert.Equal(t, "item-99", got.AggregateID)
	assert.Equal(t, "item.stocked.v1", got.EventType)
	assert.Equal(t, "trace-xyz", got.TraceID)
}
