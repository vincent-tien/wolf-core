package concurrency_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/vincent-tien/wolf-core/infra/concurrency"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestWorkerPool_ProcessesAllItems(t *testing.T) {
	t.Parallel()

	// Arrange
	const itemCount = 20
	var processed atomic.Int32

	handler := func(_ context.Context, _ int) error {
		processed.Add(1)
		return nil
	}
	pool := concurrency.NewWorkerPool(4, itemCount, handler, nil)

	// Act
	pool.Start(context.Background())
	for i := range itemCount {
		require.NoError(t, pool.Submit(i))
	}
	pool.Stop()

	// Assert
	assert.Equal(t, int32(itemCount), processed.Load())
}

func TestWorkerPool_SubmitToFullQueueReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange — queue size 1, no workers started so nothing is consumed
	pool := concurrency.NewWorkerPool(1, 1, func(_ context.Context, _ int) error {
		return nil
	}, nil)

	// Act — fill the queue then submit one more
	err1 := pool.Submit(1)
	err2 := pool.Submit(2)

	// Cleanup — start and stop to avoid leaking goroutines
	pool.Start(context.Background())
	pool.Stop()

	// Assert
	assert.NoError(t, err1, "first submit should succeed")
	assert.Error(t, err2, "second submit to full queue should fail")
	assert.Contains(t, err2.Error(), "queue is full")
}

func TestWorkerPool_StopDrainsRemainingItems(t *testing.T) {
	t.Parallel()

	// Arrange — pre-fill the queue before starting workers so items are
	// already buffered when Stop is called.
	const itemCount = 10
	var processed atomic.Int32

	// Use a slow handler to ensure items are still in-flight at Stop time.
	handler := func(_ context.Context, _ int) error {
		time.Sleep(5 * time.Millisecond)
		processed.Add(1)
		return nil
	}
	pool := concurrency.NewWorkerPool(2, itemCount, handler, nil)

	// Submit all items before starting (queue is buffered).
	for i := range itemCount {
		require.NoError(t, pool.Submit(i))
	}

	// Act — start then immediately stop; Stop must drain the queue.
	pool.Start(context.Background())
	pool.Stop()

	// Assert
	assert.Equal(t, int32(itemCount), processed.Load())
}

func TestWorkerPool_MultipleWorkersConcurrency(t *testing.T) {
	t.Parallel()

	// Arrange — verify that multiple workers execute concurrently by tracking
	// the peak observed concurrency via an atomic counter.
	const (
		workers   = 4
		itemCount = workers * 3
	)

	var (
		current atomic.Int32
		peak    atomic.Int32
	)

	handler := func(_ context.Context, _ int) error {
		n := current.Add(1)
		// Update peak if this is higher.
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond) // hold the "slot" briefly
		current.Add(-1)
		return nil
	}

	pool := concurrency.NewWorkerPool(workers, itemCount, handler, nil)

	// Act
	pool.Start(context.Background())
	for i := range itemCount {
		require.NoError(t, pool.Submit(i))
	}
	pool.Stop()

	// Assert — with 4 workers and enough items, peak concurrency must exceed 1.
	assert.Greater(t, peak.Load(), int32(1), "workers should run concurrently")
}

func TestWorkerPool_ErrorHandlerCalledOnHandlerError(t *testing.T) {
	t.Parallel()

	// Arrange
	handlerErr := errors.New("handler failure")
	var (
		mu          sync.Mutex
		capturedErr error
	)

	onError := func(err error) {
		mu.Lock()
		capturedErr = err
		mu.Unlock()
	}

	handler := func(_ context.Context, _ int) error {
		return handlerErr
	}

	pool := concurrency.NewWorkerPool(1, 4, handler, onError)

	// Act
	pool.Start(context.Background())
	require.NoError(t, pool.Submit(42))
	pool.Stop()

	// Assert
	mu.Lock()
	err := capturedErr
	mu.Unlock()
	require.Error(t, err)
	assert.Equal(t, handlerErr, err)
}

func TestWorkerPool_NilOnErrorSilencesErrors(t *testing.T) {
	t.Parallel()

	// Arrange — nil onError must not panic when handler returns an error.
	handler := func(_ context.Context, _ int) error {
		return errors.New("silent error")
	}
	pool := concurrency.NewWorkerPool(1, 4, handler, nil)

	// Act + Assert — no panic
	pool.Start(context.Background())
	require.NoError(t, pool.Submit(1))
	pool.Stop()
}

func TestWorkerPool_DefaultsForInvalidArguments(t *testing.T) {
	t.Parallel()

	// Arrange — zero workers and zero queue size fall back to safe defaults.
	var processed atomic.Int32
	handler := func(_ context.Context, _ int) error {
		processed.Add(1)
		return nil
	}

	pool := concurrency.NewWorkerPool(0, 0, handler, nil)

	// Act
	pool.Start(context.Background())
	require.NoError(t, pool.Submit(1))
	pool.Stop()

	// Assert
	assert.Equal(t, int32(1), processed.Load())
}
