// Package decorator provides a generic function decorator pattern with
// composable middleware for cross-cutting concerns such as caching, logging,
// and metrics. It mirrors the functional middleware style used in shared/cqrs
// but operates on plain functions rather than handler interfaces.
package decorator

import "context"

// Func is a generic function signature that accepts a context and an input of
// type In, returning an output of type Out or an error.
type Func[In, Out any] func(ctx context.Context, in In) (Out, error)

// Middleware wraps a Func to add cross-cutting behavior.
type Middleware[In, Out any] func(Func[In, Out]) Func[In, Out]

// Chain applies middlewares to fn from last to first so that the first
// middleware in the slice is the outermost wrapper (first to execute).
// An empty middleware list returns fn unchanged.
func Chain[In, Out any](fn Func[In, Out], mws ...Middleware[In, Out]) Func[In, Out] {
	for i := len(mws) - 1; i >= 0; i-- {
		fn = mws[i](fn)
	}
	return fn
}
