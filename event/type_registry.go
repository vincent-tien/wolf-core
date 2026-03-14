// type_registry.go — Runtime type registry for event payload serialization.
//
// Problem: the outbox worker stores event payloads as JSON blobs. When a
// subscriber receives an event, it needs to deserialize the JSON back into
// the correct Go struct. Without a registry, this requires a giant type
// switch or unsafe reflection guesswork.
//
// Solution: each module registers its event types during bootstrap via
// module.RegisterEvents(r *TypeRegistry). The registry maps event type
// strings (e.g. "iam.user.registered.v1") to reflect.Type, enabling
// Build() to create zero-value instances and Deserialize() to unmarshal.
//
// Thread safety: all methods are guarded by sync.RWMutex. Registration
// happens once during bootstrap (write lock); serialization happens on
// every request (read lock).
package event

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// TypeRegistry maps event type strings to Go types, enabling serialization
// and deserialization of event payloads without type switches. Thread-safe.
type TypeRegistry struct {
	mu    sync.RWMutex
	types map[string]reflect.Type
}

// NewTypeRegistry creates an empty TypeRegistry.
func NewTypeRegistry() *TypeRegistry {
	return &TypeRegistry{
		types: make(map[string]reflect.Type),
	}
}

// Register associates an event type string with the Go type of exemplar.
// Exemplar should be a pointer to the payload struct (e.g., &ProductCreatedPayload{}).
// Panics if eventType is already registered with a different type.
func (r *TypeRegistry) Register(eventType string, exemplar any) {
	t := reflect.TypeOf(exemplar)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.types[eventType]; ok && existing != t {
		panic(fmt.Sprintf("event: type %q already registered as %s, cannot re-register as %s",
			eventType, existing, t))
	}
	r.types[eventType] = t
}

// Build creates a new zero-value instance of the type registered for eventType.
// Returns a pointer to the new instance. Returns an error if the type is not registered.
func (r *TypeRegistry) Build(eventType string) (any, error) {
	r.mu.RLock()
	t, ok := r.types[eventType]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("event: type %q not registered", eventType)
	}
	return reflect.New(t).Interface(), nil
}

// Serialize marshals a payload to JSON. Returns an error if the event type
// is not registered or marshalling fails.
func (r *TypeRegistry) Serialize(eventType string, payload any) ([]byte, error) {
	r.mu.RLock()
	_, ok := r.types[eventType]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("event: type %q not registered", eventType)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("event: serialize %q: %w", eventType, err)
	}
	return data, nil
}

// Deserialize unmarshals JSON data into a new instance of the registered type.
// Returns a pointer to the deserialized payload.
func (r *TypeRegistry) Deserialize(eventType string, data []byte) (any, error) {
	instance, err := r.Build(eventType)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, instance); err != nil {
		return nil, fmt.Errorf("event: deserialize %q: %w", eventType, err)
	}
	return instance, nil
}

// IsRegistered reports whether the given event type has been registered.
func (r *TypeRegistry) IsRegistered(eventType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.types[eventType]
	return ok
}
