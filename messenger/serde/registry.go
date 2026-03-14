// registry.go — Type registry mapping message type names + versions to Go types.
package serde

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// Upcaster transforms a message payload from an older version to a newer version.
type Upcaster func(old json.RawMessage) (json.RawMessage, error)

// TypeRegistry maps message type names + versions to Go types for deserialization.
type TypeRegistry struct {
	mu        sync.RWMutex
	types     map[string]map[int]reflect.Type // name → version → Go type
	upcasters map[string]map[int]Upcaster     // name → fromVersion → transform
}

// NewTypeRegistry creates an empty type registry.
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		types:     make(map[string]map[int]reflect.Type),
		upcasters: make(map[string]map[int]Upcaster),
	}
}

// RegisterMessage registers a message type T with its canonical name and version.
// The canonical name comes from Typer interface or reflect.
func RegisterMessage[T any](r *TypeRegistry, name string, version int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.types[name]; !ok {
		r.types[name] = make(map[int]reflect.Type)
	}
	r.types[name][version] = reflect.TypeOf((*T)(nil)).Elem()
}

// RegisterUpcaster registers an upcaster from fromVersion to fromVersion+1.
func (r *TypeRegistry) RegisterUpcaster(name string, fromVersion int, fn Upcaster) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.upcasters[name]; !ok {
		r.upcasters[name] = make(map[int]Upcaster)
	}
	r.upcasters[name][fromVersion] = fn
}

// LatestType returns the Go type and version for the latest registered version.
func (r *TypeRegistry) LatestType(name string) (reflect.Type, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.types[name]
	if !ok {
		return nil, 0, fmt.Errorf("serde: unknown message type %q", name)
	}

	maxVer := 0
	var maxType reflect.Type
	for v, t := range versions {
		if v > maxVer {
			maxVer = v
			maxType = t
		}
	}
	return maxType, maxVer, nil
}

// GetUpcaster returns the upcaster for the given name and fromVersion.
func (r *TypeRegistry) GetUpcaster(name string, fromVersion int) (Upcaster, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, ok := r.upcasters[name]
	if !ok {
		return nil, false
	}
	fn, ok := versions[fromVersion]
	return fn, ok
}

// Upcast applies the upcaster chain from fromVersion to latestVersion.
func (r *TypeRegistry) Upcast(name string, data json.RawMessage, fromVersion, toVersion int) (json.RawMessage, error) {
	current := data
	for v := fromVersion; v < toVersion; v++ {
		fn, ok := r.GetUpcaster(name, v)
		if !ok {
			return nil, fmt.Errorf("serde: no upcaster for %q from version %d", name, v)
		}
		var err error
		current, err = fn(current)
		if err != nil {
			return nil, fmt.Errorf("serde: upcaster failed for %q v%d→v%d: %w", name, v, v+1, err)
		}
	}
	return current, nil
}
