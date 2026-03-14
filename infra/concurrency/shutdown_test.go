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

	"github.com/vincent-tien/wolf-core/infra/concurrency"
)

func TestShutdownGroup_SingleCloserExecutes(t *testing.T) {
	t.Parallel()

	// Arrange
	var called bool
	var g concurrency.ShutdownGroup
	g.AddFunc(0, "resource-a", func(_ context.Context) error {
		called = true
		return nil
	})

	// Act
	err := g.Shutdown(context.Background())

	// Assert
	require.NoError(t, err)
	assert.True(t, called)
}

func TestShutdownGroup_PriorityOrdering(t *testing.T) {
	t.Parallel()

	// Arrange — record the order in which closers are called.
	var (
		mu    sync.Mutex
		order []string
	)
	record := func(name string) func(context.Context) error {
		return func(_ context.Context) error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return nil
		}
	}

	var g concurrency.ShutdownGroup
	g.AddFunc(2, "low-priority", record("low"))
	g.AddFunc(0, "high-priority", record("high"))
	g.AddFunc(1, "mid-priority", record("mid"))

	// Act
	err := g.Shutdown(context.Background())

	// Assert
	require.NoError(t, err)
	mu.Lock()
	got := make([]string, len(order))
	copy(got, order)
	mu.Unlock()

	require.Equal(t, []string{"high", "mid", "low"}, got)
}

func TestShutdownGroup_SamePriorityRunsConcurrently(t *testing.T) {
	t.Parallel()

	// Arrange — two closers at the same priority each sleep briefly.
	// If they ran sequentially the total time would exceed their combined
	// sleep; if concurrent, it should complete in roughly one sleep cycle.
	const sleepDur = 50 * time.Millisecond

	var g concurrency.ShutdownGroup
	for range 2 {
		g.AddFunc(0, "concurrent", func(_ context.Context) error {
			time.Sleep(sleepDur)
			return nil
		})
	}

	// Act
	start := time.Now()
	err := g.Shutdown(context.Background())
	elapsed := time.Since(start)

	// Assert — should finish in less than two full sleeps.
	require.NoError(t, err)
	assert.Less(t, elapsed, sleepDur*2,
		"same-priority closers must run concurrently")
}

func TestShutdownGroup_ErrorsCollectedFromAllClosers(t *testing.T) {
	t.Parallel()

	// Arrange
	errA := errors.New("closer-a failed")
	errB := errors.New("closer-b failed")

	var g concurrency.ShutdownGroup
	g.AddFunc(0, "a", func(_ context.Context) error { return errA })
	g.AddFunc(0, "b", func(_ context.Context) error { return errB })
	g.AddFunc(1, "c", func(_ context.Context) error { return nil })

	// Act
	err := g.Shutdown(context.Background())

	// Assert — both errors must be present in the joined error.
	require.Error(t, err)
	assert.True(t, errors.Is(err, errA), "errA must be wrapped")
	assert.True(t, errors.Is(err, errB), "errB must be wrapped")
}

func TestShutdownGroup_EmptyGroupReturnsNil(t *testing.T) {
	t.Parallel()

	var g concurrency.ShutdownGroup

	err := g.Shutdown(context.Background())

	assert.NoError(t, err)
}

func TestShutdownGroup_ContextCancellationPropagatedToClosers(t *testing.T) {
	t.Parallel()

	// Arrange — closer checks whether ctx is already cancelled.
	var receivedCtxErr error
	var g concurrency.ShutdownGroup
	g.AddFunc(0, "ctx-aware", func(ctx context.Context) error {
		receivedCtxErr = ctx.Err()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Shutdown

	// Act
	err := g.Shutdown(ctx)

	// Assert
	require.NoError(t, err) // the closer itself returned nil
	assert.Equal(t, context.Canceled, receivedCtxErr,
		"closer must receive the cancelled context")
}

func TestShutdownGroup_ShutdownContinuesAfterFailure(t *testing.T) {
	t.Parallel()

	// Arrange — priority-0 closer fails; priority-1 closer must still run.
	var secondCalled atomic.Bool
	var g concurrency.ShutdownGroup
	g.AddFunc(0, "failing", func(_ context.Context) error {
		return errors.New("boom")
	})
	g.AddFunc(1, "subsequent", func(_ context.Context) error {
		secondCalled.Store(true)
		return nil
	})

	// Act
	err := g.Shutdown(context.Background())

	// Assert
	require.Error(t, err)
	assert.True(t, secondCalled.Load(), "second closer must run despite prior failure")
}

func TestShutdownGroup_CloserFunc_ImplementsCloser(t *testing.T) {
	t.Parallel()

	// Arrange — verify CloserFunc satisfies the Closer interface.
	var called bool
	fn := concurrency.CloserFunc(func(_ context.Context) error {
		called = true
		return nil
	})

	var g concurrency.ShutdownGroup
	g.Add(0, "fn-closer", fn)

	// Act
	err := g.Shutdown(context.Background())

	// Assert
	require.NoError(t, err)
	assert.True(t, called)
}
