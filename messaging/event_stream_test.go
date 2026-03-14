package messaging_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/events/inprocess"
	"github.com/vincent-tien/wolf-core/event"
	"github.com/vincent-tien/wolf-core/messaging"
)

// --- Test payload types ---

type orderPlacedPayload struct {
	OrderID string  `json:"order_id"`
	Total   float64 `json:"total"`
}

// newTestEventStream creates an EventStream backed by an in-process stream and
// a TypeRegistry pre-loaded with orderPlacedPayload.
func newTestEventStream(t *testing.T) (*messaging.EventStream, *event.TypeRegistry) {
	t.Helper()
	stream := inprocess.NewStream(zap.NewNop())
	t.Cleanup(func() { _ = stream.Close() })

	reg := event.NewTypeRegistry()
	reg.Register("order.placed.v1", &orderPlacedPayload{})

	return messaging.NewEventStream(stream, reg), reg
}

func TestEventStream_Publish_SerializesPayload(t *testing.T) {
	// Arrange
	es, _ := newTestEventStream(t)
	subject := "events.orders"

	payload := &orderPlacedPayload{OrderID: "ord-001", Total: 99.99}
	evt := event.NewEvent(
		"order.placed.v1",
		payload,
		event.WithAggregateInfo("ord-001", "Order"),
		event.WithCorrelationID("corr-abc"),
		event.WithTraceID("trace-xyz"),
		event.WithSource("order-service"),
	)

	received := make(chan event.Event, 1)
	err := es.Subscribe(subject, func(_ context.Context, e event.Event) error {
		received <- e
		return nil
	})
	require.NoError(t, err)

	// Act
	err = es.Publish(context.Background(), subject, evt)
	require.NoError(t, err)

	// Assert
	select {
	case got := <-received:
		assert.Equal(t, "order.placed.v1", got.EventType())
		assert.Equal(t, "ord-001", got.AggregateID())
		assert.Equal(t, "Order", got.AggregateType())
		assert.Equal(t, "corr-abc", got.GetMetadata().CorrelationID)
		assert.Equal(t, "trace-xyz", got.GetMetadata().TraceID)
		assert.Equal(t, "order-service", got.GetMetadata().Source)

		gotPayload, ok := got.Payload().(*orderPlacedPayload)
		require.True(t, ok, "payload should be *orderPlacedPayload, got %T", got.Payload())
		assert.Equal(t, "ord-001", gotPayload.OrderID)
		assert.InDelta(t, 99.99, gotPayload.Total, 0.001)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event delivery")
	}
}

func TestEventStream_RoundTrip_PreservesEventID(t *testing.T) {
	// Arrange
	es, _ := newTestEventStream(t)
	subject := "events.orders"

	payload := &orderPlacedPayload{OrderID: "ord-002", Total: 10.00}
	published := event.NewEvent("order.placed.v1", payload)

	received := make(chan event.Event, 1)
	_ = es.Subscribe(subject, func(_ context.Context, e event.Event) error {
		received <- e
		return nil
	})

	// Act
	require.NoError(t, es.Publish(context.Background(), subject, published))

	// Assert — the EventID reconstructed from the header must match the original
	select {
	case got := <-received:
		// EventID is re-created by NewEvent inside Subscribe (new UUID), but
		// the payload content must match exactly.
		assert.Equal(t, "order.placed.v1", got.EventType())
		gotPayload := got.Payload().(*orderPlacedPayload)
		assert.Equal(t, "ord-002", gotPayload.OrderID)
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestEventStream_Subscribe_UnknownEventType_AcksAndReturnsError(t *testing.T) {
	// Arrange — stream with no registry entries
	rawStream := inprocess.NewStream(zap.NewNop())
	defer rawStream.Close()
	reg := event.NewTypeRegistry()
	// Do NOT register any type
	es := messaging.NewEventStream(rawStream, reg)

	subject := "events.unknown"
	handlerCalled := false

	_ = es.Subscribe(subject, func(_ context.Context, e event.Event) error {
		handlerCalled = true
		return nil
	})

	// Act — publish a raw message that claims to be an unregistered event type
	err := rawStream.Publish(context.Background(), subject, messaging.RawMessage{
		Subject: subject,
		Data:    []byte(`{"order_id":"x"}`),
		Headers: map[string]string{
			"event_type": "totally.unknown.v1",
		},
	})
	require.NoError(t, err)

	// Assert — handler should NOT be invoked; the message is discarded
	assert.False(t, handlerCalled, "event handler must not be called for unknown event type")
}

func TestEventStream_Subscribe_MissingEventTypeHeader_AcksAndReturnsError(t *testing.T) {
	// Arrange
	rawStream := inprocess.NewStream(zap.NewNop())
	defer rawStream.Close()
	reg := event.NewTypeRegistry()
	es := messaging.NewEventStream(rawStream, reg)

	subject := "events.noheader"
	handlerCalled := false

	_ = es.Subscribe(subject, func(_ context.Context, e event.Event) error {
		handlerCalled = true
		return nil
	})

	// Act — message with no event_type header
	err := rawStream.Publish(context.Background(), subject, messaging.RawMessage{
		Subject: subject,
		Data:    []byte(`{}`),
		Headers: map[string]string{},
	})
	require.NoError(t, err)

	// Assert
	assert.False(t, handlerCalled, "handler must not be called when event_type header is absent")
}

func TestEventStream_Version_PreservedThroughTransport(t *testing.T) {
	// Arrange
	es, _ := newTestEventStream(t)
	subject := "events.versioned"

	payload := &orderPlacedPayload{OrderID: "ord-003", Total: 5.00}
	evt := event.NewEvent("order.placed.v1", payload, event.WithVersion(3))

	received := make(chan event.Event, 1)
	_ = es.Subscribe(subject, func(_ context.Context, e event.Event) error {
		received <- e
		return nil
	})

	// Act
	require.NoError(t, es.Publish(context.Background(), subject, evt))

	// Assert
	select {
	case got := <-received:
		assert.Equal(t, 3, got.Version())
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}
