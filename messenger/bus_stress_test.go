package messenger

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

// atomicSender is a thread-safe mock sender for stress tests.
type atomicSender struct {
	count atomic.Int64
}

func (s *atomicSender) Send(_ context.Context, _ Envelope) error {
	s.count.Add(1)
	return nil
}

func TestBus_ConcurrentDispatch_100Goroutines(t *testing.T) {
	bus := NewBus("stress")
	var callCount atomic.Int64
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		callCount.Add(1)
		return nil
	})

	const goroutines = 100
	const perGoroutine = 1000

	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := range perGoroutine {
				_, err := bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "stress"})
				if err != nil {
					t.Errorf("goroutine %d dispatch %d: %v", n, j, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	if got := callCount.Load(); got != goroutines*perGoroutine {
		t.Errorf("callCount = %d, want %d", got, goroutines*perGoroutine)
	}
}

func TestBus_ConcurrentDispatchAndRouteUpdate(t *testing.T) {
	sender := &atomicSender{}
	router := NewRouter()
	bus := NewBus("stress",
		WithRouter(router),
		WithTransport("nats", sender),
	)
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// 50 reader goroutines dispatching continuously.
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				_, _ = bus.Dispatch(ctx, placeOrderCmd{OrderID: "stress"})
			}
		}()
	}

	// 1 writer goroutine toggling routes.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			router.UpdateRoutes(map[string]string{
				"order.PlaceOrderCmd": "nats",
			})
			time.Sleep(5 * time.Millisecond)
			router.UpdateRoutes(map[string]string{})
			time.Sleep(5 * time.Millisecond)
		}
	}()

	wg.Wait()
	// If we get here without -race alerting or panic: PASS.
}

func TestBus_ConcurrentDispatchWithForceStamps(t *testing.T) {
	sender := &atomicSender{}
	router := NewRouterFromMap(map[string]string{
		"order.PlaceOrderCmd": "nats",
	})
	bus := NewBus("stress",
		WithRouter(router),
		WithTransport("nats", sender),
	)
	var syncCount atomic.Int64
	RegisterCommandFunc[placeOrderCmd](bus.Handlers(), func(_ context.Context, _ placeOrderCmd) error {
		syncCount.Add(1)
		return nil
	})

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				// Alternate between async and force-sync dispatch.
				_, _ = bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "async"})
				_, _ = bus.Dispatch(context.Background(), placeOrderCmd{OrderID: "sync"}, stamp.ForceSyncStamp{})
			}
		}()
	}

	wg.Wait()

	if got := syncCount.Load(); got != 50*1000 {
		t.Errorf("sync handler calls = %d, want %d", got, 50*1000)
	}
	if got := sender.count.Load(); got != 50*1000 {
		t.Errorf("async sends = %d, want %d", got, 50*1000)
	}
}
