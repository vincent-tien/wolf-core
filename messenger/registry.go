// registry.go — Handler registration and resolution for the messenger bus.
//
// HandlerRegistry maps message type names to their handlers (command or query).
// Uses the same lock-free read / copy-on-write pattern as Router for zero
// contention on the dispatch hot path.
//
// Registration happens at module startup via RegisterCommand[C] or
// RegisterQuery[Q,R]. Each message type can have exactly one handler —
// duplicate registration panics (programming error caught at startup).
//
// Generic registration functions (RegisterCommand, RegisterQuery) wrap
// typed handlers into an anyHandler interface, enabling the Bus to dispatch
// without knowing concrete types at compile time.
package messenger

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// handlerTable is an immutable snapshot of registered handlers.
// Replaced atomically on write — reads are lock-free.
type handlerTable struct {
	handlers map[string]anyHandler
}

// HandlerRegistry provides lock-free handler resolution on the hot path.
//
// Read path: atomic.Pointer.Load() + map lookup — zero locks, zero contention.
// Write path: copy-on-write under mutex — only at startup/config reload.
type HandlerRegistry struct {
	table atomic.Pointer[handlerTable]
	mu    sync.Mutex // protects writes only
}

// NewHandlerRegistry creates an empty registry.
func NewHandlerRegistry() *HandlerRegistry {
	r := &HandlerRegistry{}
	r.table.Store(&handlerTable{handlers: make(map[string]anyHandler)})
	return r
}

// register adds a handler to the registry under typeName.
// Panics on duplicate (programming error at startup).
func (r *HandlerRegistry) register(typeName string, h anyHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current := r.table.Load()
	if _, exists := current.handlers[typeName]; exists {
		panic(fmt.Sprintf("%v: %s", ErrDuplicateHandler, typeName))
	}

	newMap := make(map[string]anyHandler, len(current.handlers)+1)
	for k, v := range current.handlers {
		newMap[k] = v
	}
	newMap[typeName] = h
	r.table.Store(&handlerTable{handlers: newMap})
}

// RegisterCommand registers a command handler for type C.
// Panics on duplicate registration (programming error at startup).
func RegisterCommand[C any](reg *HandlerRegistry, handler CommandHandler[C]) {
	PreregisterType[C]()
	reg.register(TypeNameOf(*new(C)), &commandWrapper[C]{handler: handler})
}

// RegisterCommandFunc registers a command handler function for type C.
func RegisterCommandFunc[C any](reg *HandlerRegistry, fn func(ctx context.Context, cmd C) error) {
	RegisterCommand(reg, CommandHandlerFunc[C](fn))
}

// RegisterQuery registers a query handler for type Q returning R.
// Panics on duplicate registration.
func RegisterQuery[Q any, R any](reg *HandlerRegistry, handler QueryHandler[Q, R]) {
	PreregisterType[Q]()
	reg.register(TypeNameOf(*new(Q)), &queryWrapper[Q, R]{handler: handler})
}

// RegisterQueryFunc registers a query handler function for type Q returning R.
func RegisterQueryFunc[Q any, R any](reg *HandlerRegistry, fn func(ctx context.Context, query Q) (R, error)) {
	RegisterQuery(reg, QueryHandlerFunc[Q, R](fn))
}

// Resolve returns the handler for msg's type. Lock-free hot path.
func (r *HandlerRegistry) Resolve(msg any) (anyHandler, error) {
	t := r.table.Load()
	typeName := TypeNameOf(msg)
	h, ok := t.handlers[typeName]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoHandler, typeName)
	}
	return h, nil
}

// Has returns true if a handler is registered for msg's type.
func (r *HandlerRegistry) Has(msg any) bool {
	t := r.table.Load()
	_, ok := t.handlers[TypeNameOf(msg)]
	return ok
}

// RegisteredTypes returns all registered type names (for debugging).
func (r *HandlerRegistry) RegisteredTypes() []string {
	t := r.table.Load()
	types := make([]string, 0, len(t.handlers))
	for k := range t.handlers {
		types = append(types, k)
	}
	return types
}

// CommandTypes returns type names of registered command handlers.
func (r *HandlerRegistry) CommandTypes() []string {
	t := r.table.Load()
	var types []string
	for k, h := range t.handlers {
		if h.IsCommand() {
			types = append(types, k)
		}
	}
	return types
}

// QueryTypes returns type names of registered query handlers.
func (r *HandlerRegistry) QueryTypes() []string {
	t := r.table.Load()
	var types []string
	for k, h := range t.handlers {
		if !h.IsCommand() {
			types = append(types, k)
		}
	}
	return types
}
