// handler.go — Type-safe command and query handler contracts for the messenger bus.
//
// Design: Two handler types with fundamentally different semantics:
//   - CommandHandler[C]: fire-and-forget (no return value). Can be routed to
//     async transport (outbox, message queue) because there is no caller waiting.
//   - QueryHandler[Q,R]: request-reply (returns R). ALWAYS synchronous because
//     the caller needs the result immediately.
//
// Both are stored in the bus registry via type-erased wrappers (anyHandler).
// The wrappers perform runtime type assertion on dispatch and return a clear
// error if the message type doesn't match the registered handler.
//
// When adding a new handler: implement CommandHandler[YourCmd] or
// QueryHandler[YourQuery, YourResult], then register it on the bus with
// RegisterCommandFunc/RegisterQueryFunc.
package messenger

import (
	"context"
	"fmt"
)

// CommandHandler handles a command with no return value.
// This is the ONLY handler type that can be routed to async transport.
type CommandHandler[C any] interface {
	Handle(ctx context.Context, cmd C) error
}

// CommandHandlerFunc adapts a function to CommandHandler.
type CommandHandlerFunc[C any] func(ctx context.Context, cmd C) error

func (f CommandHandlerFunc[C]) Handle(ctx context.Context, cmd C) error {
	return f(ctx, cmd)
}

// QueryHandler handles a query and returns a result. ALWAYS synchronous.
type QueryHandler[Q any, R any] interface {
	Handle(ctx context.Context, query Q) (R, error)
}

// QueryHandlerFunc adapts a function to QueryHandler.
type QueryHandlerFunc[Q any, R any] func(ctx context.Context, query Q) (R, error)

func (f QueryHandlerFunc[Q, R]) Handle(ctx context.Context, query Q) (R, error) {
	return f(ctx, query)
}

// anyHandler is the type-erased internal interface for storage in registry map.
type anyHandler interface {
	HandleAny(ctx context.Context, msg any) (any, error)
	IsCommand() bool
}

// commandWrapper type-erases CommandHandler[C] for registry storage.
type commandWrapper[C any] struct {
	handler CommandHandler[C]
}

func (w *commandWrapper[C]) HandleAny(ctx context.Context, msg any) (any, error) {
	cmd, ok := msg.(C)
	if !ok {
		return nil, fmt.Errorf("messenger: type assertion failed: expected %T, got %T", *new(C), msg)
	}
	return nil, w.handler.Handle(ctx, cmd)
}

func (w *commandWrapper[C]) IsCommand() bool { return true }

// queryWrapper type-erases QueryHandler[Q,R] for registry storage.
type queryWrapper[Q any, R any] struct {
	handler QueryHandler[Q, R]
}

func (w *queryWrapper[Q, R]) HandleAny(ctx context.Context, msg any) (any, error) {
	query, ok := msg.(Q)
	if !ok {
		return nil, fmt.Errorf("messenger: type assertion failed: expected %T, got %T", *new(Q), msg)
	}
	result, err := w.handler.Handle(ctx, query)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (w *queryWrapper[Q, R]) IsCommand() bool { return false }
