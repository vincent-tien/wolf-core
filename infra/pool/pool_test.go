package pool_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vincent-tien/wolf-core/infra/pool"
)

func TestObjectPool_GetReturnsNewObject(t *testing.T) {
	t.Parallel()

	// Arrange
	type payload struct{ value int }
	factoryCalls := 0
	p := pool.NewObjectPool(func() *payload {
		factoryCalls++
		return &payload{value: 42}
	})

	// Act
	obj := p.Get()

	// Assert
	require.NotNil(t, obj)
	assert.Equal(t, 42, obj.value)
	assert.Equal(t, 1, factoryCalls)
}

func TestObjectPool_PutAndReuse(t *testing.T) {
	t.Parallel()

	// Arrange
	type payload struct{ id int }
	p := pool.NewObjectPool(func() *payload {
		return &payload{id: 0}
	})

	first := p.Get()
	first.id = 99

	// Act — return to pool and immediately re-acquire (likely same object before GC)
	p.Put(first)
	second := p.Get()

	// Assert — we assert no panic and a valid object is returned.
	// sync.Pool may discard items between GC cycles, so reuse is best-effort.
	require.NotNil(t, second)
}

func TestObjectPool_ConcurrentSafety(t *testing.T) {
	t.Parallel()

	// Arrange
	const goroutines = 200

	p := pool.NewObjectPool(func() *[]byte {
		b := make([]byte, 128)
		return &b
	})

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Act
	for range goroutines {
		go func() {
			defer wg.Done()

			obj := p.Get()
			require.NotNil(t, obj)
			// Simulate some usage.
			(*obj)[0] = 1
			p.Put(obj)
		}()
	}

	// Assert — no race or panic; verified by -race flag and successful completion.
	wg.Wait()
}
