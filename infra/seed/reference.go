// reference.go — Thread-safe typed data sharing between seeders.
package seed

import (
	"fmt"
	"sync"
)

// ReferenceStore provides typed, cross-seeder data sharing. A seeder that
// creates entities (e.g. roles) stores their IDs so downstream seeders
// (e.g. admin_user) can reference them without querying the database.
type ReferenceStore struct {
	mu   sync.RWMutex
	data map[string]any
}

// NewReferenceStore creates an empty reference store.
func NewReferenceStore() *ReferenceStore {
	return &ReferenceStore{data: make(map[string]any)}
}

// Set stores a value under the given key.
func (rs *ReferenceStore) Set(key string, value any) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.data[key] = value
}

// Get retrieves a raw value by key. Returns false if the key does not exist.
func (rs *ReferenceStore) Get(key string) (any, bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	v, ok := rs.data[key]
	return v, ok
}

// MustGet retrieves a raw value by key and panics if the key does not exist.
func (rs *ReferenceStore) MustGet(key string) any {
	v, ok := rs.Get(key)
	if !ok {
		panic(fmt.Sprintf("seed: reference %q not found", key))
	}
	return v
}

// Keys returns all stored reference keys.
func (rs *ReferenceStore) Keys() []string {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	keys := make([]string, 0, len(rs.data))
	for k := range rs.data {
		keys = append(keys, k)
	}
	return keys
}

// GetRef is a generic accessor that retrieves and type-asserts a reference.
// It returns an error if the key is missing or the type assertion fails.
func GetRef[T any](rs *ReferenceStore, key string) (T, error) {
	var zero T
	raw, ok := rs.Get(key)
	if !ok {
		return zero, fmt.Errorf("seed: reference %q not found", key)
	}
	typed, ok := raw.(T)
	if !ok {
		return zero, fmt.Errorf("seed: reference %q has type %T, want %T", key, raw, zero)
	}
	return typed, nil
}

// MustGetRef is like GetRef but panics on error.
func MustGetRef[T any](rs *ReferenceStore, key string) T {
	v, err := GetRef[T](rs, key)
	if err != nil {
		panic(err.Error())
	}
	return v
}
