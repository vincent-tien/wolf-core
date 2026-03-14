// pool.go — Generic type-safe sync.Pool wrapper.
package pool

import "sync"

// ObjectPool is a generic, type-safe wrapper around sync.Pool.
type ObjectPool[T any] struct {
	pool sync.Pool
}

// NewObjectPool creates a pool with the given factory function.
// factory is called when the pool is empty and a new object is needed.
func NewObjectPool[T any](factory func() T) *ObjectPool[T] {
	return &ObjectPool[T]{
		pool: sync.Pool{
			New: func() any { return factory() },
		},
	}
}

// Get retrieves an object from the pool or creates a new one via the factory.
func (p *ObjectPool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put returns an object to the pool for reuse.
func (p *ObjectPool[T]) Put(x T) {
	p.pool.Put(x)
}
