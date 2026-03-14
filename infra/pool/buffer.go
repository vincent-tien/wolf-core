// buffer.go — Pooled bytes.Buffer with size-limited return to reduce GC pressure.
package pool

import (
	"bytes"
	"sync"
)

// DefaultMaxBufferSize is the maximum buffer size that will be returned to the pool.
// Buffers larger than this are discarded to prevent unbounded memory growth.
const DefaultMaxBufferSize = 64 * 1024 // 64 KB

// BufferPool manages a pool of *bytes.Buffer with a size guard.
// Buffers exceeding MaxSize are not returned to the pool.
type BufferPool struct {
	pool    sync.Pool
	maxSize int
}

// NewBufferPool creates a BufferPool. Buffers larger than maxSize bytes
// are discarded on Put instead of being returned to the pool.
// If maxSize is 0, DefaultMaxBufferSize is used.
func NewBufferPool(maxSize int) *BufferPool {
	if maxSize <= 0 {
		maxSize = DefaultMaxBufferSize
	}

	bp := &BufferPool{maxSize: maxSize}
	bp.pool = sync.Pool{
		New: func() any { return new(bytes.Buffer) },
	}

	return bp
}

// Get retrieves a buffer from the pool, reset to zero length.
func (bp *BufferPool) Get() *bytes.Buffer {
	buf := bp.pool.Get().(*bytes.Buffer)
	buf.Reset()

	return buf
}

// Put returns a buffer to the pool if its capacity does not exceed maxSize.
// Oversized buffers are discarded to prevent memory bloat.
func (bp *BufferPool) Put(buf *bytes.Buffer) {
	if buf == nil {
		return
	}

	if buf.Cap() > bp.maxSize {
		return
	}

	buf.Reset()
	bp.pool.Put(buf)
}
