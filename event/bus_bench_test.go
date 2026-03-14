// Package event_test contains benchmarks for the event bus implementations.
package event_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/vincent-tien/wolf-core/event"
)

// BenchmarkInMemoryBus_Publish measures the per-Publish cost of the
// InMemoryBus as the number of registered handlers grows. Each sub-benchmark
// registers n no-op handlers for a single event type, then times repeated
// Publish calls to expose the linear fanout cost.
func BenchmarkInMemoryBus_Publish(b *testing.B) {
	for _, n := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("handlers=%d", n), func(b *testing.B) {
			// Arrange
			bus := event.NewInMemoryBus()
			for range n {
				bus.Subscribe("test.event", func(_ context.Context, _ event.Event) error {
					return nil
				})
			}
			evt := &stubEvent{id: "bench-1", eventType: "test.event"}
			ctx := context.Background()

			b.ResetTimer()
			for range b.N {
				_ = bus.Publish(ctx, evt)
			}
		})
	}
}
