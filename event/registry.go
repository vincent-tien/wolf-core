// Package event defines the core domain event contracts for the wolf-be platform.
package event

// HandlerRegistry collects event handler registrations and applies them
// to a Subscriber in a single batch. This enables declarative event wiring
// in module.go instead of imperative bus.Subscribe calls.
type HandlerRegistry struct {
	entries []registryEntry
}

type registryEntry struct {
	eventType string
	handler   EventHandler
}

// NewHandlerRegistry creates an empty HandlerRegistry.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{}
}

// Register adds a handler for the given event type to the registry.
func (r *HandlerRegistry) Register(eventType string, handler EventHandler) {
	r.entries = append(r.entries, registryEntry{
		eventType: eventType,
		handler:   handler,
	})
}

// RegisterAll applies all collected registrations to the given subscriber.
// This is typically called from module.RegisterSubscribers().
func (r *HandlerRegistry) RegisterAll(sub Subscriber) {
	for _, e := range r.entries {
		sub.Subscribe(e.eventType, e.handler)
	}
}

// Len returns the number of registered handlers.
func (r *HandlerRegistry) Len() int {
	return len(r.entries)
}
