package messenger

import (
	"context"
	"errors"
	"testing"

	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

// ── Test types ──

type placeOrderCmd struct{ OrderID string }

func (placeOrderCmd) MessageName() string { return "order.PlaceOrderCmd" }

type getOrderQuery struct{ OrderID string }

func (getOrderQuery) MessageName() string { return "order.GetOrderQuery" }

type orderDetail struct{ OrderID, Status string }

// mockSender records Send calls for testing async dispatch.
type mockSender struct {
	sent []Envelope
	err  error
}

func (m *mockSender) Send(_ context.Context, env Envelope) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, env)
	return nil
}

// ── Sync Command Tests ──

func TestBus_SyncCommand_HandlerCalled(t *testing.T) {
	bus := NewBus("test")
	var received string
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, cmd placeOrderCmd) error {
		received = cmd.OrderID
		return nil
	})

	result, err := bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "ord-1"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.Async {
		t.Error("expected sync dispatch")
	}
	if received != "ord-1" {
		t.Errorf("received = %q, want %q", received, "ord-1")
	}
}

func TestBus_SyncQuery_ResultReturned(t *testing.T) {
	bus := NewBus("test")
	RegisterQueryFunc[getOrderQuery, orderDetail](bus.Handlers(), func(_ context.Context, q getOrderQuery) (orderDetail, error) {
		return orderDetail{OrderID: q.OrderID, Status: "confirmed"}, nil
	})

	result, err := bus.Query(context.Background(), getOrderQuery{OrderID: "ord-2"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	detail, ok := result.(orderDetail)
	if !ok {
		t.Fatalf("result type = %T, want orderDetail", result)
	}
	if detail.Status != "confirmed" {
		t.Errorf("Status = %q, want %q", detail.Status, "confirmed")
	}
}

func TestBus_Query_AsyncRoute_ReturnsError(t *testing.T) {
	sender := &mockSender{}
	router := NewRouterFromMap(map[string]string{
		"order.GetOrderQuery": "nats",
	})
	bus := NewBus("test", WithRouter(router), WithTransport("nats", sender))
	RegisterQueryFunc[getOrderQuery, orderDetail](bus.Handlers(), func(_ context.Context, _ getOrderQuery) (orderDetail, error) {
		return orderDetail{}, nil
	})

	_, err := bus.Query(context.Background(), getOrderQuery{OrderID: "ord-3"})
	if !errors.Is(err, ErrQueryCannotBeAsync) {
		t.Errorf("err = %v, want ErrQueryCannotBeAsync", err)
	}
}

func TestBus_UnknownMessage_ReturnsErrNoHandler(t *testing.T) {
	bus := NewBus("test")
	_, err := bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "ord-4"})
	if !errors.Is(err, ErrNoHandler) {
		t.Errorf("err = %v, want ErrNoHandler", err)
	}
}

// ── Stamp Tests ──

func TestBus_DispatchWithStamps(t *testing.T) {
	bus := NewBus("test")
	var envStampCount int
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		return nil
	})

	// Add middleware that inspects stamps.
	stampCheckBus := NewBus("test-stamps",
		WithHandlerRegistry(bus.Handlers()),
		WithMiddleware(MiddlewareFunc(func(ctx context.Context, env Envelope, next MiddlewareNext) (DispatchResult, error) {
			envStampCount = env.StampCount()
			return next(ctx, env)
		})),
	)

	_, err := stampCheckBus.Dispatch(context.Background(), placeOrderCmd{OrderID: "ord-5"},
		stamp.TraceStamp{TraceID: "t1", SpanID: "s1"},
	)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if envStampCount != 1 {
		t.Errorf("envStampCount = %d, want 1", envStampCount)
	}
}

// ── Async Dispatch Tests ──

func TestBus_AsyncDispatch_SendsToTransport(t *testing.T) {
	sender := &mockSender{}
	router := NewRouterFromMap(map[string]string{
		"order.PlaceOrderCmd": "nats",
	})
	bus := NewBus("test",
		WithRouter(router),
		WithTransport("nats", sender),
	)
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		t.Error("handler should NOT be called for async dispatch")
		return nil
	})

	result, err := bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "ord-6"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !result.Async {
		t.Error("expected async dispatch")
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sender.sent len = %d, want 1", len(sender.sent))
	}
}

func TestBus_AsyncDispatch_TransportNotFound_PanicsAtStartup(t *testing.T) {
	router := NewRouterFromMap(map[string]string{
		"order.PlaceOrderCmd": "nonexistent",
	})
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when route references missing transport")
		}
	}()
	NewBus("test", WithRouter(router))
}

func TestBus_ForceTransportStamp_UnknownTransport_ReturnsError(t *testing.T) {
	bus := NewBus("test")
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		return nil
	})

	_, err := bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "ord-7"},
		stamp.ForceTransportStamp{TransportName: "nonexistent"},
	)
	if !errors.Is(err, ErrTransportNotFound) {
		t.Errorf("err = %v, want ErrTransportNotFound", err)
	}
}

// ── Force Stamp Tests ──

func TestBus_ForceSyncStamp_OverridesAsyncRoute(t *testing.T) {
	sender := &mockSender{}
	router := NewRouterFromMap(map[string]string{
		"order.PlaceOrderCmd": "nats",
	})
	bus := NewBus("test",
		WithRouter(router),
		WithTransport("nats", sender),
	)
	var handlerCalled bool
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		handlerCalled = true
		return nil
	})

	result, err := bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "ord-8"},
		stamp.ForceSyncStamp{},
	)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.Async {
		t.Error("expected sync dispatch with ForceSyncStamp")
	}
	if !handlerCalled {
		t.Error("handler should be called with ForceSyncStamp")
	}
	if len(sender.sent) != 0 {
		t.Error("transport should not be called with ForceSyncStamp")
	}
}

func TestBus_ForceTransportStamp_OverridesRoute(t *testing.T) {
	natsSender := &mockSender{}
	kafkaSender := &mockSender{}
	router := NewRouterFromMap(map[string]string{
		"order.PlaceOrderCmd": "nats",
	})
	bus := NewBus("test",
		WithRouter(router),
		WithTransport("nats", natsSender),
		WithTransport("kafka", kafkaSender),
	)
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		return nil
	})

	_, err := bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "ord-9"},
		stamp.ForceTransportStamp{TransportName: "kafka"},
	)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(natsSender.sent) != 0 {
		t.Error("nats should not be called")
	}
	if len(kafkaSender.sent) != 1 {
		t.Error("kafka should be called via ForceTransportStamp")
	}
}

// ── Middleware Order Tests ──

func TestBus_MiddlewareExecutionOrder(t *testing.T) {
	var order []int
	makeMW := func(id int) Middleware {
		return MiddlewareFunc(func(ctx context.Context, env Envelope, next MiddlewareNext) (DispatchResult, error) {
			order = append(order, id)
			return next(ctx, env)
		})
	}

	bus := NewBus("test", WithMiddleware(makeMW(1), makeMW(2), makeMW(3)))
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		order = append(order, 0)
		return nil
	})

	_, err := bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "ord-10"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	expected := []int{1, 2, 3, 0}
	if len(order) != len(expected) {
		t.Fatalf("order = %v, want %v", order, expected)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %d, want %d", i, order[i], v)
		}
	}
}

func TestBus_MiddlewareShortCircuit(t *testing.T) {
	shortCircuitErr := errors.New("blocked")
	mw := MiddlewareFunc(func(_ context.Context, env Envelope, _ MiddlewareNext) (DispatchResult, error) {
		return DispatchResult{Envelope: env}, shortCircuitErr
	})

	bus := NewBus("test", WithMiddleware(mw))
	var handlerCalled bool
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		handlerCalled = true
		return nil
	})

	_, err := bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "ord-11"})
	if !errors.Is(err, shortCircuitErr) {
		t.Errorf("err = %v, want shortCircuitErr", err)
	}
	if handlerCalled {
		t.Error("handler should not be called when middleware short-circuits")
	}
}

// ── Closed Bus Test ──

func TestBus_ClosedBus_ReturnsError(t *testing.T) {
	bus := NewBus("test")
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		return nil
	})
	_ = bus.Close()

	_, err := bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "ord-12"})
	if !errors.Is(err, ErrBusClosed) {
		t.Errorf("Dispatch err = %v, want ErrBusClosed", err)
	}

	_, err = bus.Query(context.Background(), getOrderQuery{OrderID: "ord-12"})
	if !errors.Is(err, ErrBusClosed) {
		t.Errorf("Query err = %v, want ErrBusClosed", err)
	}
}
