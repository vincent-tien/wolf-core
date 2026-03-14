// async.go — Lightweight async/await for Go using generics and channels.
//
// Async[T] launches a goroutine and returns a buffered channel that receives
// exactly one Result[T]. Await[T] blocks until the result is ready or the
// context is cancelled. Useful for parallelizing independent I/O operations
// without the boilerplate of manual goroutine + WaitGroup management.
package concurrency

import (
	"context"
	"fmt"
)

// Result wraps a value and error pair from an asynchronous operation.
type Result[T any] struct {
	Value T
	Err   error
}

// Async runs fn in a goroutine and returns a channel that receives exactly one
// Result. The channel is buffered (size 1) so the goroutine never blocks even
// if the receiver abandons the channel. Panics inside fn are recovered and
// returned as errors.
func Async[T any](ctx context.Context, fn func(ctx context.Context) (T, error)) <-chan Result[T] {
	ch := make(chan Result[T], 1)

	go func() {
		defer close(ch)

		var res Result[T]
		defer func() {
			if r := recover(); r != nil {
				var zero T
				res = Result[T]{Value: zero, Err: fmt.Errorf("async: panic recovered: %v", r)}
			}
			ch <- res // single send point — safe regardless of panic timing
		}()

		if ctx.Err() != nil {
			res = Result[T]{Err: ctx.Err()}
			return
		}

		res.Value, res.Err = fn(ctx)
	}()

	return ch
}

// Await receives from an Async result channel, respecting context cancellation.
func Await[T any](ctx context.Context, ch <-chan Result[T]) (T, error) {
	select {
	case result, ok := <-ch:
		if !ok {
			var zero T
			return zero, context.Canceled
		}
		return result.Value, result.Err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}
