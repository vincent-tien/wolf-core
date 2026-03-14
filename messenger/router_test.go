package messenger

import (
	"sync"
	"testing"
)

type routedCmd struct{}

func (routedCmd) MessageName() string { return "order.CreateOrderCmd" }

type unroutedCmd struct{}

func (unroutedCmd) MessageName() string { return "order.UnroutedCmd" }

func TestRouter_Empty_ReturnsSync(t *testing.T) {
	r := NewRouter()
	if got := r.Route(routedCmd{}); got != "" {
		t.Errorf("Route() = %q, want empty (sync)", got)
	}
}

func TestRouter_ConfiguredRoute(t *testing.T) {
	r := NewRouterFromMap(map[string]string{
		"order.CreateOrderCmd": "nats",
	})
	if got := r.Route(routedCmd{}); got != "nats" {
		t.Errorf("Route() = %q, want %q", got, "nats")
	}
}

func TestRouter_UnconfiguredMessage_ReturnsSync(t *testing.T) {
	r := NewRouterFromMap(map[string]string{
		"order.CreateOrderCmd": "nats",
	})
	if got := r.Route(unroutedCmd{}); got != "" {
		t.Errorf("Route() = %q, want empty (sync)", got)
	}
}

func TestRouter_UpdateRoutes(t *testing.T) {
	r := NewRouterFromMap(map[string]string{
		"order.CreateOrderCmd": "nats",
	})
	r.UpdateRoutes(map[string]string{
		"order.CreateOrderCmd": "kafka",
	})
	if got := r.Route(routedCmd{}); got != "kafka" {
		t.Errorf("Route() = %q, want %q", got, "kafka")
	}
}

func TestRouter_AddRoute(t *testing.T) {
	r := NewRouter()
	r.AddRoute("order.CreateOrderCmd", "memory")
	if got := r.Route(routedCmd{}); got != "memory" {
		t.Errorf("Route() = %q, want %q", got, "memory")
	}
}

func TestRouter_RemoveRoute(t *testing.T) {
	r := NewRouterFromMap(map[string]string{
		"order.CreateOrderCmd": "nats",
	})
	r.RemoveRoute("order.CreateOrderCmd")
	if got := r.Route(routedCmd{}); got != "" {
		t.Errorf("Route() = %q, want empty after removal", got)
	}
}

func TestRouter_Routes_ReturnsCopy(t *testing.T) {
	r := NewRouterFromMap(map[string]string{
		"order.CreateOrderCmd": "nats",
	})
	routes := r.Routes()
	routes["order.CreateOrderCmd"] = "mutated"

	if got := r.Route(routedCmd{}); got != "nats" {
		t.Errorf("Routes() mutation leaked: got %q, want %q", got, "nats")
	}
}

func TestRouter_ConcurrentRoute(t *testing.T) {
	r := NewRouterFromMap(map[string]string{
		"order.CreateOrderCmd": "nats",
	})
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				r.Route(routedCmd{})
			}
		}()
	}
	wg.Wait()
}

// ── Benchmarks ──

func BenchmarkRouterRoute_Hit(b *testing.B) {
	r := NewRouterFromMap(map[string]string{
		"order.CreateOrderCmd": "nats",
	})
	msg := routedCmd{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.Route(msg)
	}
}

func BenchmarkRouterRoute_Miss(b *testing.B) {
	r := NewRouterFromMap(map[string]string{
		"order.CreateOrderCmd": "nats",
	})
	msg := unroutedCmd{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.Route(msg)
	}
}

func BenchmarkRouterRoute_Parallel(b *testing.B) {
	r := NewRouterFromMap(map[string]string{
		"order.CreateOrderCmd": "nats",
	})
	msg := routedCmd{}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.Route(msg)
		}
	})
}
