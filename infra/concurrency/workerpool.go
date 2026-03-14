// Package concurrency provides bounded-concurrency and ordered-shutdown
// primitives for the wolf-be platform layer.
package concurrency

import (
	"context"
	"fmt"
	"sync"
)

// WorkerPool provides bounded concurrency for processing items of type T.
// Items are submitted to a buffered channel and processed by a fixed number
// of goroutines. The pool must be started before submitting items.
type WorkerPool[T any] struct {
	workers int
	queue   chan T
	handler func(context.Context, T) error
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	onError func(error)
}

// NewWorkerPool creates a WorkerPool with the given worker count and queue size.
// handler is called for each submitted item by one of the worker goroutines.
// onError is called for each error returned by handler (may be nil for silent errors).
func NewWorkerPool[T any](workers, queueSize int, handler func(context.Context, T) error, onError func(error)) *WorkerPool[T] {
	if workers <= 0 {
		workers = 1
	}
	if queueSize <= 0 {
		queueSize = workers * 2
	}
	return &WorkerPool[T]{
		workers: workers,
		queue:   make(chan T, queueSize),
		handler: handler,
		onError: onError,
	}
}

// Start launches the worker goroutines. The pool processes items until Stop
// is called or ctx is cancelled. Must be called exactly once.
func (p *WorkerPool[T]) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)
	for range p.workers {
		p.wg.Add(1)
		go p.run(ctx)
	}
}

// run is the worker loop. It processes items from the queue until the channel
// is closed. Context cancellation causes the worker to stop mid-queue without
// draining; Stop (which closes the queue without cancelling) allows a full drain.
func (p *WorkerPool[T]) run(ctx context.Context) {
	defer p.wg.Done()
	for item := range p.queue {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := p.handler(ctx, item); err != nil && p.onError != nil {
			p.onError(err)
		}
	}
}

// Submit enqueues an item for processing. Returns an error if the queue is full.
func (p *WorkerPool[T]) Submit(item T) error {
	select {
	case p.queue <- item:
		return nil
	default:
		return fmt.Errorf("workerpool: queue is full (capacity %d)", cap(p.queue))
	}
}

// Stop closes the queue and waits for all workers to finish processing
// remaining items. It does NOT cancel the context — workers drain the queue
// before exiting. The parent context cancellation will abort in-flight items.
func (p *WorkerPool[T]) Stop() {
	close(p.queue)
	p.wg.Wait()
	if p.cancel != nil {
		p.cancel()
	}
}
