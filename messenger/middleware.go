// middleware.go — Middleware contract for the messenger dispatch pipeline.
//
// Middleware forms a layered pipeline around message dispatch, similar to
// HTTP middleware but for CQRS messages. Each middleware wraps the next,
// forming a chain built at Bus construction time (see chain.go).
//
// The chain is pre-built and immutable after construction, so the dispatch
// hot path has zero allocation. Middleware ordering is set via
// WithMiddleware() option on NewBus().
//
// Built-in middleware (in platform/) includes: recovery, timeout, validation,
// logging, and metrics. Custom middleware can add tracing, auth, caching, etc.
package messenger

import "context"

// MiddlewareNext is the function to call the next middleware in the chain.
type MiddlewareNext func(ctx context.Context, env Envelope) (DispatchResult, error)

// Middleware intercepts message dispatch.
// Implementations MUST call next() unless intentionally short-circuiting
// (e.g., validation failure, circuit breaker open).
// Returns DispatchResult by VALUE to avoid heap allocation on the sync path.
type Middleware interface {
	Handle(ctx context.Context, env Envelope, next MiddlewareNext) (DispatchResult, error)
}

// MiddlewareFunc adapts a plain function to the Middleware interface.
// Convenience for inline middleware without defining a named struct.
type MiddlewareFunc func(ctx context.Context, env Envelope, next MiddlewareNext) (DispatchResult, error)

func (f MiddlewareFunc) Handle(ctx context.Context, env Envelope, next MiddlewareNext) (DispatchResult, error) {
	return f(ctx, env, next)
}
