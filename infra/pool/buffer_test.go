package pool_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vincent-tien/wolf-core/infra/pool"
)

func TestBufferPool_GetReturnsEmptyBuffer(t *testing.T) {
	t.Parallel()

	// Arrange
	bp := pool.NewBufferPool(pool.DefaultMaxBufferSize)

	// Act
	buf := bp.Get()

	// Assert
	require.NotNil(t, buf)
	assert.Equal(t, 0, buf.Len(), "Get must return a buffer with Len() == 0")
}

func TestBufferPool_PutAndReuse(t *testing.T) {
	t.Parallel()

	// Arrange
	bp := pool.NewBufferPool(pool.DefaultMaxBufferSize)

	buf := bp.Get()
	buf.WriteString("hello, wolf")

	// Act
	bp.Put(buf)
	reused := bp.Get()

	// Assert — buffer must be reset regardless of whether it is the same allocation.
	require.NotNil(t, reused)
	assert.Equal(t, 0, reused.Len(), "buffer retrieved after Put must be reset to zero length")
}

func TestBufferPool_OversizedNotPooled(t *testing.T) {
	t.Parallel()

	// Arrange — use a tiny maxSize so we can easily exceed it.
	const maxSize = 64
	bp := pool.NewBufferPool(maxSize)

	oversized := bp.Get()
	// Write more than maxSize bytes to force a large internal allocation.
	oversized.WriteString(strings.Repeat("x", maxSize+1))
	require.Greater(t, oversized.Cap(), maxSize, "buffer should have grown beyond maxSize")

	// Act — Put should discard the oversized buffer rather than return it to the pool.
	bp.Put(oversized)
	next := bp.Get()

	// Assert — a fresh (small) buffer is returned, not the oversized one.
	require.NotNil(t, next)
	assert.Equal(t, 0, next.Len(), "next buffer must be empty")
	assert.LessOrEqual(t, next.Cap(), maxSize,
		"next buffer capacity must not exceed maxSize; oversized buffer was incorrectly pooled")
}

func TestBufferPool_ConcurrentSafety(t *testing.T) {
	t.Parallel()

	// Arrange
	const goroutines = 200

	bp := pool.NewBufferPool(pool.DefaultMaxBufferSize)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Act
	for range goroutines {
		go func() {
			defer wg.Done()

			buf := bp.Get()
			require.NotNil(t, buf)
			buf.WriteString("concurrent write")
			bp.Put(buf)
		}()
	}

	// Assert — no race or panic; verified by -race flag and successful completion.
	wg.Wait()
}

func TestBufferPool_DefaultMaxSize(t *testing.T) {
	t.Parallel()

	// Arrange — passing 0 must fall back to DefaultMaxBufferSize.
	bp := pool.NewBufferPool(0)

	// Act — Put an oversized buffer (larger than DefaultMaxBufferSize) and verify it is discarded.
	big := &bytes.Buffer{}
	// Grow the buffer well beyond 64 KB.
	big.Write(bytes.Repeat([]byte("a"), pool.DefaultMaxBufferSize+1))
	require.Greater(t, big.Cap(), pool.DefaultMaxBufferSize)

	bp.Put(big)
	next := bp.Get()

	// Assert — oversized buffer must not have been pooled.
	require.NotNil(t, next)
	assert.Equal(t, 0, next.Len())
	assert.LessOrEqual(t, next.Cap(), pool.DefaultMaxBufferSize,
		"NewBufferPool(0) must use DefaultMaxBufferSize as the size guard")
}
