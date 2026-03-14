// serde.go — Type-safe serialization/deserialization wrapping TypeRegistry.
package event

import "fmt"

// Serde provides compile-time type-safe serialization and deserialization for
// a specific event payload type T. It wraps TypeRegistry operations so callers
// never need to perform manual type assertions after Decode.
//
// Create a Serde once at startup (via NewSerde or MustSerde) and reuse it
// across goroutines; it holds no mutable state beyond the registry reference.
type Serde[T any] struct {
	registry  *TypeRegistry
	eventType string
}

// NewSerde creates a Serde for the given event type. Returns an error if
// eventType is not registered in registry. Call this during module startup
// before serving requests.
func NewSerde[T any](registry *TypeRegistry, eventType string) (*Serde[T], error) {
	if !registry.IsRegistered(eventType) {
		return nil, fmt.Errorf("event: serde: type %q not registered", eventType)
	}
	return &Serde[T]{registry: registry, eventType: eventType}, nil
}

// MustSerde creates a Serde or panics. Intended for use in init() functions or
// module startup paths where a missing registration is a programmer error.
func MustSerde[T any](registry *TypeRegistry, eventType string) *Serde[T] {
	s, err := NewSerde[T](registry, eventType)
	if err != nil {
		panic(err)
	}
	return s
}

// Encode serializes payload to JSON bytes using the TypeRegistry.
// Returns an error if marshalling fails.
func (s *Serde[T]) Encode(payload T) ([]byte, error) {
	return s.registry.Serialize(s.eventType, payload)
}

// Decode deserializes JSON bytes into a new *T using the TypeRegistry.
// Returns an error if unmarshalling fails or the registry returns an
// unexpected type (which would indicate a registry misconfiguration).
func (s *Serde[T]) Decode(data []byte) (*T, error) {
	result, err := s.registry.Deserialize(s.eventType, data)
	if err != nil {
		return nil, err
	}
	// TypeRegistry.Deserialize returns reflect.New(t).Interface(), which is *T.
	typed, ok := result.(*T)
	if !ok {
		return nil, fmt.Errorf("event: serde: unexpected type %T for %q", result, s.eventType)
	}
	return typed, nil
}

// EventType returns the event type string this Serde handles.
func (s *Serde[T]) EventType() string {
	return s.eventType
}
