// Package cqrs provides the generic command and query handler interfaces
// that form the application layer contract for the wolf-be CQRS pattern.
// Command handlers mutate state; query handlers return read-model projections.
package cqrs

import "context"

// Command is a marker interface for all command messages.
// Commands express intent to change system state and are handled exactly once.
// Concrete commands should be value types (structs) carrying all necessary input.
type Command interface{}

// Query is a marker interface for all query messages.
// Queries are read-only and may be handled multiple times without side effects.
// Concrete queries should be value types (structs) carrying filter parameters.
type Query interface{}

// CommandHandler processes a single command type C and returns result R.
// Implementations must validate the command, enforce business rules, persist
// state changes, and publish domain events.
type CommandHandler[C Command, R any] interface {
	// Handle executes the command and returns the result or an error.
	// ctx must carry the request-scoped cancellation signal and trace metadata.
	Handle(ctx context.Context, cmd C) (R, error)
}

// QueryHandler processes a single query type Q and returns result R.
// Implementations must read from the read model and never mutate state.
type QueryHandler[Q Query, R any] interface {
	// Handle executes the query and returns the result or an error.
	// ctx must carry the request-scoped cancellation signal and trace metadata.
	Handle(ctx context.Context, query Q) (R, error)
}

// CommandHandlerFunc is an adapter to allow the use of ordinary functions as CommandHandlers.
type CommandHandlerFunc[C Command, R any] func(ctx context.Context, cmd C) (R, error)

func (f CommandHandlerFunc[C, R]) Handle(ctx context.Context, cmd C) (R, error) {
	return f(ctx, cmd)
}

// QueryHandlerFunc is an adapter to allow the use of ordinary functions as QueryHandlers.
type QueryHandlerFunc[Q Query, R any] func(ctx context.Context, query Q) (R, error)

func (f QueryHandlerFunc[Q, R]) Handle(ctx context.Context, query Q) (R, error) {
	return f(ctx, query)
}

// Void is the zero-size result type for commands that produce no return value.
type Void = struct{}

// voidAdapter wraps a function returning only error into CommandHandler[C, Void].
type voidAdapter[C Command] struct {
	fn func(context.Context, C) error
}

func (a *voidAdapter[C]) Handle(ctx context.Context, cmd C) (Void, error) {
	return Void{}, a.fn(ctx, cmd)
}

// AsVoidCommand adapts a void-returning handler (e.g. Logout, ChangePassword) into a
// CommandHandler[C, Void] so it can participate in the standard CQRS middleware chain.
func AsVoidCommand[C Command](fn func(context.Context, C) error) CommandHandler[C, Void] {
	return &voidAdapter[C]{fn: fn}
}

// CommandMiddleware wraps a CommandHandler to add cross-cutting concerns.
type CommandMiddleware[C Command, R any] func(CommandHandler[C, R]) CommandHandler[C, R]

// QueryMiddleware wraps a QueryHandler to add cross-cutting concerns.
type QueryMiddleware[Q Query, R any] func(QueryHandler[Q, R]) QueryHandler[Q, R]

// ChainCommand applies middlewares to a handler from bottom to top (last middleware wraps outermost).
func ChainCommand[C Command, R any](handler CommandHandler[C, R], mws ...CommandMiddleware[C, R]) CommandHandler[C, R] {
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}

// ChainQuery applies middlewares to a handler from bottom to top.
func ChainQuery[Q Query, R any](handler QueryHandler[Q, R], mws ...QueryMiddleware[Q, R]) QueryHandler[Q, R] {
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}
