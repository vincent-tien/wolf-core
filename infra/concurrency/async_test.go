package concurrency_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/concurrency"
)

func TestAsync_ReturnsValue(t *testing.T) {
	t.Parallel()

	ch := concurrency.Async(context.Background(), func(_ context.Context) (int, error) {
		return 42, nil
	})

	result, err := concurrency.Await(context.Background(), ch)
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}

func TestAsync_ReturnsError(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	ch := concurrency.Async(context.Background(), func(_ context.Context) (int, error) {
		return 0, want
	})

	_, err := concurrency.Await(context.Background(), ch)
	assert.ErrorIs(t, err, want)
}

func TestAsync_CancelledContextBeforeStart(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := concurrency.Async(ctx, func(_ context.Context) (int, error) {
		return 42, nil
	})

	_, err := concurrency.Await(context.Background(), ch)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestAwait_CancelledContextDuringWait(t *testing.T) {
	t.Parallel()

	// Use a shared context so both Async and Await see the cancellation,
	// preventing goroutine leaks detected by goleak.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	ch := concurrency.Async(ctx, func(ctx context.Context) (int, error) {
		select {
		case <-time.After(5 * time.Second):
			return 0, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	})

	_, err := concurrency.Await(ctx, ch)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestAsync_BufferedChannelPreventsLeak(t *testing.T) {
	t.Parallel()

	// Start async but never Await — goroutine must still complete (no leak).
	_ = concurrency.Async(context.Background(), func(_ context.Context) (int, error) {
		return 1, nil
	})

	// Give the goroutine time to complete. goleak in TestMain catches leaks.
	time.Sleep(50 * time.Millisecond)
}
