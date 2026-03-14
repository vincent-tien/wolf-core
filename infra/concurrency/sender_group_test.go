package concurrency_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/vincent-tien/wolf-core/infra/concurrency"
)

func TestSenderGroup_MultipleSendersCloseOnce(t *testing.T) {
	t.Parallel()

	sg := concurrency.NewSenderGroup[int](context.Background(), 10)

	sg.Go(func(_ context.Context, send func(int)) {
		send(1)
		send(2)
	})
	sg.Go(func(_ context.Context, send func(int)) {
		send(3)
		send(4)
	})
	sg.Start()

	var results []int
	for v := range sg.Channel() {
		results = append(results, v)
	}

	sort.Ints(results)
	assert.Equal(t, []int{1, 2, 3, 4}, results)
}

func TestSenderGroup_CancelledContextStopsSenders(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sg := concurrency.NewSenderGroup[int](ctx, 0)
	sg.Go(func(ctx context.Context, send func(int)) {
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				send(i)
			}
		}
	})
	sg.Start()

	// Consume a few values then cancel.
	count := 0
	for range sg.Channel() {
		count++
		if count >= 3 {
			cancel()
			break
		}
	}

	// Drain remaining values.
	for range sg.Channel() {
	}

	assert.GreaterOrEqual(t, count, 3)
}

func TestSenderGroup_ZeroSendersClosesImmediately(t *testing.T) {
	t.Parallel()

	sg := concurrency.NewSenderGroup[int](context.Background(), 0)
	sg.Start()

	select {
	case _, ok := <-sg.Channel():
		assert.False(t, ok, "channel should be closed immediately")
	case <-time.After(time.Second):
		t.Fatal("channel was not closed")
	}
}
