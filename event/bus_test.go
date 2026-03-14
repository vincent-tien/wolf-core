// Package event defines the core domain event contracts for the wolf-be platform.
package event

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryBus_SubscribeAndPublish(t *testing.T) {
	bus := NewInMemoryBus()
	var received Event

	bus.Subscribe("order.created", func(_ context.Context, evt Event) error {
		received = evt
		return nil
	})

	evt := NewEvent("order.created", nil, WithAggregateInfo("order-1", "Order"))
	err := bus.Publish(context.Background(), evt)

	require.NoError(t, err)
	require.NotNil(t, received)
	assert.Equal(t, "order.created", received.EventType())
	assert.Equal(t, "order-1", received.AggregateID())
}

func TestInMemoryBus_NoSubscribers_NoError(t *testing.T) {
	bus := NewInMemoryBus()

	evt := NewEvent("order.created", nil, WithAggregateInfo("order-1", "Order"))
	err := bus.Publish(context.Background(), evt)

	assert.NoError(t, err)
}

func TestInMemoryBus_MultipleHandlers_AllCalled(t *testing.T) {
	bus := NewInMemoryBus()
	callCount := 0

	bus.Subscribe("x.y", func(_ context.Context, _ Event) error { callCount++; return nil })
	bus.Subscribe("x.y", func(_ context.Context, _ Event) error { callCount++; return nil })
	bus.Subscribe("x.y", func(_ context.Context, _ Event) error { callCount++; return nil })

	evt := NewEvent("x.y", nil)
	require.NoError(t, bus.Publish(context.Background(), evt))
	assert.Equal(t, 3, callCount)
}

func TestInMemoryBus_HandlerError_StopsChain(t *testing.T) {
	bus := NewInMemoryBus()
	secondCalled := false

	bus.Subscribe("x.y", func(_ context.Context, _ Event) error {
		return errors.New("first handler failed")
	})
	bus.Subscribe("x.y", func(_ context.Context, _ Event) error {
		secondCalled = true
		return nil
	})

	evt := NewEvent("x.y", nil)
	err := bus.Publish(context.Background(), evt)

	require.Error(t, err)
	assert.False(t, secondCalled, "second handler must not be called after error")
}

func TestInMemoryBus_DifferentEventTypes_Isolated(t *testing.T) {
	bus := NewInMemoryBus()
	orderHandled := false
	paymentHandled := false

	bus.Subscribe("order.created", func(_ context.Context, _ Event) error {
		orderHandled = true
		return nil
	})
	bus.Subscribe("payment.processed", func(_ context.Context, _ Event) error {
		paymentHandled = true
		return nil
	})

	orderEvt := NewEvent("order.created", nil)
	require.NoError(t, bus.Publish(context.Background(), orderEvt))

	assert.True(t, orderHandled)
	assert.False(t, paymentHandled, "payment handler must not fire for order event")
}

func TestInMemoryBus_Close_NoError(t *testing.T) {
	bus := NewInMemoryBus()
	assert.NoError(t, bus.Close())
}

func TestInMemoryBus_ImplementsBusInterface(t *testing.T) {
	var _ Bus = NewInMemoryBus()
}
