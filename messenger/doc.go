// Package messenger provides a unified CQRS message dispatching system.
//
// It consolidates command dispatching and query handling into a single Bus
// interface with configurable sync/async routing. Default behavior is
// synchronous with zero allocation overhead. Async dispatch is opt-in via
// Router configuration, requiring no code changes to business logic.
//
// Key types:
//   - Bus / MessageBus: dispatch entry point (Dispatch for commands, Query for queries)
//   - Envelope: immutable value carrying message + stamps through the chain
//   - Middleware: intercepts dispatch (logging, validation, metrics, recovery)
//   - Router: maps message types to sync or async transports
//   - HandlerRegistry: resolves message types to command/query handlers
//   - BusSet: manages separate command/query bus instances
//
// Design inspired by Symfony Messenger, adapted for Go's type system
// and performance characteristics.
package messenger
