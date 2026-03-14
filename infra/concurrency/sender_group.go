// sender_group.go — Multi-sender channel with automatic close on completion.
package concurrency

import (
	"context"
	"sync"
)

// SenderGroup coordinates multiple goroutines writing to a single channel.
// It closes the output channel automatically after all senders finish,
// preventing "send on closed channel" panics.
type SenderGroup[T any] struct {
	ch  chan T
	wg  sync.WaitGroup
	ctx context.Context
}

// NewSenderGroup creates a channel managed by multiple senders.
// capacity controls the channel buffer size (0 for unbuffered).
func NewSenderGroup[T any](ctx context.Context, capacity int) *SenderGroup[T] {
	return &SenderGroup[T]{
		ch:  make(chan T, capacity),
		ctx: ctx,
	}
}

// Go spawns a sender goroutine. send writes to the shared channel with
// context-cancellation protection. Register all Go calls before Start.
func (sg *SenderGroup[T]) Go(fn func(ctx context.Context, send func(T))) {
	sg.wg.Go(func() {
		fn(sg.ctx, func(val T) {
			select {
			case sg.ch <- val:
			case <-sg.ctx.Done():
			}
		})
	})
}

// Channel returns the read-only side for the receiver.
func (sg *SenderGroup[T]) Channel() <-chan T {
	return sg.ch
}

// Start waits for all senders in a background goroutine and closes the
// channel. Call this after all Go calls are registered.
func (sg *SenderGroup[T]) Start() {
	go func() {
		sg.wg.Wait()
		close(sg.ch)
	}()
}
