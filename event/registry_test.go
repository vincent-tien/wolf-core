package event_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/event"
)

// mockSubscriber records every Subscribe call for assertion in tests.
type mockSubscriber struct {
	subscriptions []struct {
		eventType string
		handler   event.EventHandler
	}
}

func (m *mockSubscriber) Subscribe(eventType string, handler event.EventHandler) {
	m.subscriptions = append(m.subscriptions, struct {
		eventType string
		handler   event.EventHandler
	}{eventType, handler})
}

// nopHandler is a no-op EventHandler used where the handler body is irrelevant.
func nopHandler(_ context.Context, _ event.Event) error { return nil }

func TestNewHandlerRegistry_StartsEmpty(t *testing.T) {
	r := event.NewHandlerRegistry()

	assert.Equal(t, 0, r.Len())
}

func TestHandlerRegistry_Register_IncrementsLen(t *testing.T) {
	r := event.NewHandlerRegistry()

	r.Register("order.created", nopHandler)
	assert.Equal(t, 1, r.Len())

	r.Register("order.confirmed", nopHandler)
	assert.Equal(t, 2, r.Len())

	r.Register("user.registered", nopHandler)
	assert.Equal(t, 3, r.Len())
}

func TestHandlerRegistry_RegisterAll_CallsSubscribeForEachEntry(t *testing.T) {
	r := event.NewHandlerRegistry()
	r.Register("order.created", nopHandler)
	r.Register("order.confirmed", nopHandler)
	r.Register("user.registered", nopHandler)

	sub := &mockSubscriber{}
	r.RegisterAll(sub)

	require.Len(t, sub.subscriptions, 3)
	assert.Equal(t, "order.created", sub.subscriptions[0].eventType)
	assert.Equal(t, "order.confirmed", sub.subscriptions[1].eventType)
	assert.Equal(t, "user.registered", sub.subscriptions[2].eventType)
}

func TestHandlerRegistry_MultipleHandlersSameEventType_AllRegistered(t *testing.T) {
	r := event.NewHandlerRegistry()
	r.Register("order.created", nopHandler)
	r.Register("order.created", nopHandler)
	r.Register("order.created", nopHandler)

	assert.Equal(t, 3, r.Len())

	sub := &mockSubscriber{}
	r.RegisterAll(sub)

	require.Len(t, sub.subscriptions, 3)
	for _, s := range sub.subscriptions {
		assert.Equal(t, "order.created", s.eventType)
	}
}

func TestHandlerRegistry_RegisterAll_EmptyRegistry_IsNoOp(t *testing.T) {
	r := event.NewHandlerRegistry()

	sub := &mockSubscriber{}
	r.RegisterAll(sub)

	assert.Empty(t, sub.subscriptions)
}

func TestHandlerRegistry_RegisterAll_HandlerFunctionsArePreserved(t *testing.T) {
	callCount := 0
	countingHandler := func(_ context.Context, _ event.Event) error {
		callCount++
		return nil
	}

	r := event.NewHandlerRegistry()
	r.Register("payment.processed", countingHandler)

	bus := event.NewInMemoryBus()
	r.RegisterAll(bus)

	evt := event.NewEvent("payment.processed", nil, event.WithAggregateInfo("pay-1", "Payment"))
	err := bus.Publish(context.Background(), evt)

	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "registered handler must be invoked on publish")
}
