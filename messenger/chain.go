// chain.go — Pre-built middleware chain for zero-allocation per-request dispatch.
package messenger

import "context"

// terminalFunc is the final handler in a middleware chain.
type terminalFunc func(ctx context.Context, env Envelope) (DispatchResult, error)

// chainNode is a single node in the pre-built middleware chain.
// Created ONCE at startup, reused for every request — zero per-request allocation.
//
// Key insight: nextFunc is a pre-bound MiddlewareNext stored at build time.
// This avoids creating a bound method value (n.next.execute) per request,
// which would allocate a closure on every dispatch.
type chainNode struct {
	middleware Middleware
	nextFunc   MiddlewareNext // pre-built at startup — 1 alloc per node at build, 0 at runtime
	terminal   terminalFunc   // non-nil only on the tail node
}

// execute runs this node in the chain.
func (n *chainNode) execute(ctx context.Context, env Envelope) (DispatchResult, error) {
	if n.terminal != nil {
		return n.terminal(ctx, env)
	}
	return n.middleware.Handle(ctx, env, n.nextFunc)
}

// buildChain creates a pre-built middleware chain from a slice of middleware and a terminal.
// All allocations happen here (once at startup). The per-request hot path is zero-alloc.
//
// Execution order: middleware[0] → middleware[1] → ... → middleware[N-1] → terminal
func buildChain(mws []Middleware, terminal terminalFunc) *chainNode {
	tail := &chainNode{terminal: terminal}

	if len(mws) == 0 {
		return tail
	}

	// Build from tail to head. Each node captures a pre-bound nextFunc
	// pointing to the next node's execute method — allocated once here.
	current := tail
	for i := len(mws) - 1; i >= 0; i-- {
		next := current
		current = &chainNode{
			middleware: mws[i],
			nextFunc:   next.execute, // bound method value allocated once at build time
		}
	}
	return current
}
