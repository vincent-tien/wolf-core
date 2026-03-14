package event

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventDispatcher_Dispatch_CallsHandlersInOrder(t *testing.T) {
	d := NewEventDispatcher[string]()

	var log []string
	d.Register(func(_ context.Context, _ Event, p string) error {
		log = append(log, "h1:"+p)
		return nil
	})
	d.Register(func(_ context.Context, _ Event, p string) error {
		log = append(log, "h2:"+p)
		return nil
	})

	evt := NewEvent("test.event.v1", "hello")
	err := d.Dispatch(context.Background(), evt)

	require.NoError(t, err)
	assert.Equal(t, []string{"h1:hello", "h2:hello"}, log)
}

func TestEventDispatcher_Dispatch_StopsOnError(t *testing.T) {
	d := NewEventDispatcher[string]()

	var called bool
	d.Register(func(_ context.Context, _ Event, _ string) error {
		return fmt.Errorf("boom")
	})
	d.Register(func(_ context.Context, _ Event, _ string) error {
		called = true
		return nil
	})

	evt := NewEvent("test.event.v1", "x")
	err := d.Dispatch(context.Background(), evt)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	assert.False(t, called, "second handler must not run after first error")
}

func TestEventDispatcher_Dispatch_WrongPayloadType_ReturnsError(t *testing.T) {
	d := NewEventDispatcher[int]()
	d.Register(func(_ context.Context, _ Event, _ int) error { return nil })

	evt := NewEvent("test.event.v1", "wrong-type")
	err := d.Dispatch(context.Background(), evt)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected payload type")
}

func TestEventDispatcher_Dispatch_PointerPayload_Dereferenced(t *testing.T) {
	d := NewEventDispatcher[string]()

	var got string
	d.Register(func(_ context.Context, _ Event, p string) error {
		got = p
		return nil
	})

	val := "ptr-value"
	evt := NewEvent("test.event.v1", &val)
	err := d.Dispatch(context.Background(), evt)

	require.NoError(t, err)
	assert.Equal(t, "ptr-value", got)
}

func TestEventDispatcher_ConcurrentDispatch_NoRace(t *testing.T) {
	d := NewEventDispatcher[int]()
	var count atomic.Int64

	d.Register(func(_ context.Context, _ Event, _ int) error {
		count.Add(1)
		return nil
	})

	const goroutines = 100
	var wg sync.WaitGroup

	for i := range goroutines {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			evt := NewEvent("test.event.v1", n)
			_ = d.Dispatch(context.Background(), evt)
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int64(goroutines), count.Load())
}

func TestEventDispatcher_RegisterDuringDispatch_NoRace(t *testing.T) {
	d := NewEventDispatcher[int]()
	d.Register(func(_ context.Context, _ Event, _ int) error { return nil })

	var wg sync.WaitGroup

	// Dispatch concurrently.
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			evt := NewEvent("test.event.v1", 1)
			_ = d.Dispatch(context.Background(), evt)
		}()
	}

	// Register concurrently.
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Register(func(_ context.Context, _ Event, _ int) error { return nil })
		}()
	}

	wg.Wait()
}

func TestEventDispatcher_AsEventHandler_BridgesTyped(t *testing.T) {
	d := NewEventDispatcher[string]()

	var got string
	d.Register(func(_ context.Context, _ Event, p string) error {
		got = p
		return nil
	})

	handler := d.AsEventHandler()
	evt := NewEvent("test.event.v1", "bridged")
	err := handler(context.Background(), evt)

	require.NoError(t, err)
	assert.Equal(t, "bridged", got)
}
