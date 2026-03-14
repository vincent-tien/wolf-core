// Package event_test contains external tests for the event bus contract.
package event_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/event"
)

// stubEvent is a minimal Event implementation used exclusively in contract tests.
type stubEvent struct {
	id        string
	eventType string
}

func (e *stubEvent) EventID() string       { return e.id }
func (e *stubEvent) EventType() string     { return e.eventType }
func (e *stubEvent) AggregateID() string   { return "agg-1" }
func (e *stubEvent) AggregateType() string { return "Test" }
func (e *stubEvent) OccurredAt() time.Time { return time.Now().UTC() }
func (e *stubEvent) Version() int          { return 1 }
func (e *stubEvent) Payload() any          { return nil }
func (e *stubEvent) GetMetadata() event.Metadata {
	return event.Metadata{}
}

// RunBusContractSuite executes the full contract test suite against any Bus
// implementation. Call this from a concrete implementation test to verify
// compliance with the Bus interface contract.
func RunBusContractSuite(t *testing.T, factory func() event.Bus) {
	t.Helper()

	t.Run("Subscribe_and_Publish_delivers_event_to_handler", func(t *testing.T) {
		t.Parallel()

		// Arrange
		bus := factory()
		var received event.Event
		bus.Subscribe("test.created", func(_ context.Context, evt event.Event) error {
			received = evt
			return nil
		})
		evt := &stubEvent{id: "evt-1", eventType: "test.created"}

		// Act
		err := bus.Publish(context.Background(), evt)

		// Assert
		require.NoError(t, err)
		require.NotNil(t, received)
		assert.Equal(t, "test.created", received.EventType())
		assert.Equal(t, "evt-1", received.EventID())
	})

	t.Run("Multiple_handlers_for_same_event_type_all_receive_event", func(t *testing.T) {
		t.Parallel()

		// Arrange
		bus := factory()
		callCount := 0
		for range 3 {
			bus.Subscribe("test.created", func(_ context.Context, _ event.Event) error {
				callCount++
				return nil
			})
		}
		evt := &stubEvent{id: "evt-2", eventType: "test.created"}

		// Act
		err := bus.Publish(context.Background(), evt)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, 3, callCount, "all three handlers must be invoked")
	})

	t.Run("Handler_for_different_event_type_does_not_receive_event", func(t *testing.T) {
		t.Parallel()

		// Arrange
		bus := factory()
		wrongHandlerCalled := false
		bus.Subscribe("other.event", func(_ context.Context, _ event.Event) error {
			wrongHandlerCalled = true
			return nil
		})
		evt := &stubEvent{id: "evt-3", eventType: "test.created"}

		// Act
		err := bus.Publish(context.Background(), evt)

		// Assert
		require.NoError(t, err)
		assert.False(t, wrongHandlerCalled, "handler registered for a different event type must not fire")
	})

	t.Run("First_handler_error_aborts_remaining_handlers_and_is_returned", func(t *testing.T) {
		t.Parallel()

		// Arrange
		bus := factory()
		sentinel := errors.New("first handler failed")
		secondCalled := false

		bus.Subscribe("test.created", func(_ context.Context, _ event.Event) error {
			return sentinel
		})
		bus.Subscribe("test.created", func(_ context.Context, _ event.Event) error {
			secondCalled = true
			return nil
		})
		evt := &stubEvent{id: "evt-4", eventType: "test.created"}

		// Act
		err := bus.Publish(context.Background(), evt)

		// Assert
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)
		assert.False(t, secondCalled, "second handler must not be called after first error")
	})

	t.Run("Publish_with_no_handlers_is_a_no_op", func(t *testing.T) {
		t.Parallel()

		// Arrange
		bus := factory()
		evt := &stubEvent{id: "evt-5", eventType: "test.created"}

		// Act
		err := bus.Publish(context.Background(), evt)

		// Assert
		assert.NoError(t, err, "publishing with no subscribers must not return an error")
	})

	t.Run("Close_returns_nil", func(t *testing.T) {
		t.Parallel()

		// Arrange
		bus := factory()

		// Act
		err := bus.Close()

		// Assert
		assert.NoError(t, err)
	})
}

// TestInMemoryBus_ContractSuite verifies the InMemoryBus satisfies the full Bus contract.
func TestInMemoryBus_ContractSuite(t *testing.T) {
	t.Parallel()
	RunBusContractSuite(t, func() event.Bus {
		return event.NewInMemoryBus()
	})
}
