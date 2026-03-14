// shutdown.go — Priority-based graceful shutdown orchestrator.
//
// ShutdownGroup coordinates teardown of all application resources (servers,
// DB pools, caches, message brokers) in a defined order. Resources are
// grouped by integer priority (lower = earlier). Within the same priority,
// resources shut down concurrently via goroutines + WaitGroup.
//
// Typical priority assignment (see bootstrap/lifecycle.go):
//   0: HTTP/gRPC servers (stop accepting traffic)
//   1: modules (flush in-flight work)
//   2: event bus (close subscriptions)
//   3: cache client (close Redis connections)
//   4: DB connection pools
//   5: tracer provider (flush remaining spans)
package concurrency

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// Closer is a resource that can be shut down.
type Closer interface {
	Close(ctx context.Context) error
}

// CloserFunc adapts a plain function to the Closer interface.
type CloserFunc func(ctx context.Context) error

// Close implements Closer.
func (f CloserFunc) Close(ctx context.Context) error { return f(ctx) }

// ShutdownGroup manages ordered teardown of resources grouped by priority.
// Resources with the same priority are shut down concurrently.
// Lower priority numbers execute first (priority 0 before priority 1).
type ShutdownGroup struct {
	mu      sync.Mutex
	entries []shutdownEntry
}

type shutdownEntry struct {
	priority int
	name     string
	closer   Closer
}

// Add registers a resource for shutdown at the given priority.
// Lower numbers shut down first. Thread-safe.
func (g *ShutdownGroup) Add(priority int, name string, closer Closer) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.entries = append(g.entries, shutdownEntry{
		priority: priority,
		name:     name,
		closer:   closer,
	})
}

// AddFunc is a convenience method that wraps a function as a Closer.
func (g *ShutdownGroup) AddFunc(priority int, name string, fn func(ctx context.Context) error) {
	g.Add(priority, name, CloserFunc(fn))
}

// Shutdown executes all registered closers in priority order.
// Same-priority closers run concurrently. All errors are collected and
// returned as a joined error. Shutdown continues even when a step fails
// so that subsequent resources are still released.
func (g *ShutdownGroup) Shutdown(ctx context.Context) error {
	g.mu.Lock()
	entries := make([]shutdownEntry, len(g.entries))
	copy(entries, g.entries)
	g.mu.Unlock()

	// Stable sort preserves registration order within the same priority.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].priority < entries[j].priority
	})

	var allErrs []error

	i := 0
	for i < len(entries) {
		// Identify the contiguous slice that shares the current priority.
		currentPriority := entries[i].priority
		j := i
		for j < len(entries) && entries[j].priority == currentPriority {
			j++
		}

		// Execute the batch concurrently.
		batch := entries[i:j]
		batchErrs := make([]error, len(batch))

		var wg sync.WaitGroup
		for idx, entry := range batch {
			wg.Add(1)
			go func(idx int, e shutdownEntry) {
				defer wg.Done()
				batchErrs[idx] = e.closer.Close(ctx)
			}(idx, entry)
		}
		wg.Wait()

		for _, err := range batchErrs {
			if err != nil {
				allErrs = append(allErrs, err)
			}
		}

		i = j
	}

	return errors.Join(allErrs...)
}
