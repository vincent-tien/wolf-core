// errors.go — Sentinel errors for the messenger bus.
//
// All errors use the "messenger:" prefix for easy identification in logs.
// Use errors.Is() to check these at call sites — never compare by string.
package messenger

import "errors"

var (
	// ErrNoHandler is returned when no handler is registered for a message type.
	ErrNoHandler = errors.New("messenger: no handler registered for message type")

	// ErrQueryCannotBeAsync is returned when a query type is found in async routing table.
	ErrQueryCannotBeAsync = errors.New("messenger: query types cannot be routed to async transport")

	// ErrTransportNotFound is returned when a transport name is not registered.
	ErrTransportNotFound = errors.New("messenger: transport not found")

	// ErrBusClosed is returned when dispatch is called after bus shutdown.
	ErrBusClosed = errors.New("messenger: bus is closed")

	// ErrDuplicateHandler is returned when a handler is already registered for a message type.
	ErrDuplicateHandler = errors.New("messenger: duplicate handler for message type")

	// ErrCircuitOpen is returned when the circuit breaker is open for a transport.
	ErrCircuitOpen = errors.New("messenger: circuit breaker open")
)
