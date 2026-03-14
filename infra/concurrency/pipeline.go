// pipeline.go — Fan-out/fan-in primitives for concurrent stream processing.
package concurrency

import (
	"context"
	"sync"
)

// Stage is a function that transforms an input value into an output value.
// It is used as a building block for composable pipeline architectures.
type Stage[In, Out any] func(ctx context.Context, in In) (Out, error)

// FanOut distributes items received from in across n worker goroutines, each
// running fn. Results are sent to the returned channel. The output channel is
// closed when all workers finish. Errors from fn are silently dropped; callers
// that need error propagation should encode errors into Out (e.g. use a
// result wrapper type).
//
// If ctx is cancelled, workers stop reading from in and drain completes once
// in-flight work finishes.
func FanOut[T, R any](ctx context.Context, in <-chan T, workers int, fn Stage[T, R]) <-chan R {
	if workers <= 0 {
		workers = 1
	}

	out := make(chan R, workers)
	var wg sync.WaitGroup

	for range workers {
		wg.Go(func() {
			for item := range in {
				if ctx.Err() != nil {
					return
				}
				result, err := fn(ctx, item)
				if err != nil {
					continue
				}
				select {
				case out <- result:
				case <-ctx.Done():
					return
				}
			}
		})
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

// FanIn merges multiple input channels into a single output channel. The
// output channel is closed when every input channel has been drained.
func FanIn[T any](ctx context.Context, channels ...<-chan T) <-chan T {
	out := make(chan T, len(channels))
	var wg sync.WaitGroup

	for _, ch := range channels {
		wg.Go(func() {
			for item := range ch {
				select {
				case out <- item:
				case <-ctx.Done():
					return
				}
			}
		})
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
