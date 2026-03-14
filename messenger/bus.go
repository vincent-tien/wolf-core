// bus.go — Core MessageBus: the unified CQRS dispatch entry point.
//
// MessageBus replaces direct handler wiring with a single dispatch surface
// for both commands and queries. It routes messages through a pre-built
// middleware chain (zero-alloc on sync path), then either:
//   - Executes the handler synchronously (default, queries always)
//   - Sends to a transport for async processing (commands only, opt-in via Router)
//
// Dispatch flow:
//   Dispatch(cmd) → middleware chain → Router decision:
//     ├── sync:  resolve handler → execute → return result
//     └── async: send to transport → return {Async: true}
//
// Query(q) always runs synchronously — async queries are rejected with
// ErrQueryCannotBeAsync.
//
// Stamps on the envelope carry metadata through the chain (tracing, routing
// overrides, results). See stamp/ package for the full set of stamp types.
package messenger

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

// Sender sends an envelope to an external transport (outbox, NATS, etc.).
// Defined here (not in a transport package) to avoid circular imports.
type Sender interface {
	Send(ctx context.Context, env Envelope) error
}

// Bus is the unified CQRS dispatch interface.
// Dispatch handles commands (sync or async). Query handles queries (sync only).
type Bus interface {
	Dispatch(ctx context.Context, cmd any, stamps ...stamp.Stamp) (DispatchResult, error)
	Query(ctx context.Context, query any, stamps ...stamp.Stamp) (any, error)
	Handlers() *HandlerRegistry
}

var _ Bus = (*MessageBus)(nil)

// MessageBus is the default Bus implementation.
// Pre-built chains ensure zero allocation on the sync dispatch hot path.
type MessageBus struct {
	name       string
	handlers   *HandlerRegistry
	router     *Router
	transports map[string]Sender
	chain      *chainNode // command dispatch chain
	queryChain *chainNode // query dispatch chain (no async routing)
	closed     atomic.Bool
}

// NewBus creates a MessageBus with the given options.
// Middleware chains are pre-built here — zero allocation on the hot path.
func NewBus(name string, opts ...BusOption) *MessageBus {
	b := &MessageBus{
		name:       name,
		handlers:   NewHandlerRegistry(),
		router:     NewRouter(),
		transports: make(map[string]Sender),
	}

	var mws []Middleware
	for _, opt := range opts {
		opt.apply(b, &mws)
	}

	// Fail fast: verify routes reference registered transports.
	for msgType, transportName := range b.router.Routes() {
		if _, ok := b.transports[transportName]; !ok {
			panic(fmt.Sprintf(
				"messenger: route %q → transport %q not registered (available: %v)",
				msgType, transportName, transportNames(b.transports),
			))
		}
	}

	syncTerminal := b.makeSyncTerminal()
	asyncTerminal := b.makeDispatchTerminal(syncTerminal)

	b.chain = buildChain(mws, asyncTerminal)
	b.queryChain = buildChain(mws, syncTerminal)
	return b
}

func transportNames(m map[string]Sender) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	return names
}

// Dispatch sends a command through the middleware chain.
// With 0 stamps and sync routing, this is zero-allocation.
func (b *MessageBus) Dispatch(ctx context.Context, cmd any, stamps ...stamp.Stamp) (DispatchResult, error) {
	if b.closed.Load() {
		return DispatchResult{}, ErrBusClosed
	}
	env := NewEnvelope(cmd, stamps...)
	return b.chain.execute(ctx, env)
}

// Query dispatches a query synchronously through the middleware chain.
// Queries CANNOT be routed to async transports — returns ErrQueryCannotBeAsync.
func (b *MessageBus) Query(ctx context.Context, query any, stamps ...stamp.Stamp) (any, error) {
	if b.closed.Load() {
		return nil, ErrBusClosed
	}

	if transport := b.router.Route(query); transport != "" {
		return nil, fmt.Errorf("%w: %s routed to %q", ErrQueryCannotBeAsync, TypeNameOf(query), transport)
	}

	env := NewEnvelope(query, stamps...)
	result, err := b.queryChain.execute(ctx, env)
	if err != nil {
		return nil, err
	}

	if rs := result.Envelope.Last(stamp.NameResult); rs != nil {
		return rs.(stamp.ResultStamp).Value, nil
	}
	return nil, nil
}

// Handlers returns the handler registry for external registration.
func (b *MessageBus) Handlers() *HandlerRegistry {
	return b.handlers
}

// Close shuts down the bus. Further dispatches return ErrBusClosed.
func (b *MessageBus) Close() error {
	b.closed.Store(true)
	return nil
}

// makeSyncTerminal creates the terminal function for synchronous dispatch.
// Resolves handler and calls it. Attaches ResultStamp for query results.
func (b *MessageBus) makeSyncTerminal() terminalFunc {
	return func(ctx context.Context, env Envelope) (DispatchResult, error) {
		handler, err := b.handlers.Resolve(env.Message)
		if err != nil {
			return DispatchResult{}, err
		}

		result, err := handler.HandleAny(ctx, env.Message)
		if err != nil {
			return DispatchResult{}, err
		}

		if result != nil {
			env = env.WithStamp(stamp.ResultStamp{Value: result})
		}
		return DispatchResult{Envelope: env}, nil
	}
}

// makeDispatchTerminal creates the terminal for command dispatch.
// Checks router: if async route exists, sends to transport; otherwise falls through to sync.
func (b *MessageBus) makeDispatchTerminal(syncTerminal terminalFunc) terminalFunc {
	return func(ctx context.Context, env Envelope) (DispatchResult, error) {
		// Check for ForceSyncStamp override.
		if env.HasStamp(stamp.NameForceSync) {
			return syncTerminal(ctx, env)
		}

		// Check for ForceTransportStamp override.
		if fs := env.Last(stamp.NameForceTransport); fs != nil {
			transportName := fs.(stamp.ForceTransportStamp).TransportName
			return b.sendToTransport(ctx, env, transportName)
		}

		// Normal routing.
		transportName := b.router.Route(env.Message)
		if transportName == "" {
			return syncTerminal(ctx, env)
		}
		return b.sendToTransport(ctx, env, transportName)
	}
}

func (b *MessageBus) sendToTransport(ctx context.Context, env Envelope, transportName string) (DispatchResult, error) {
	sender, ok := b.transports[transportName]
	if !ok {
		return DispatchResult{}, fmt.Errorf("%w: %q", ErrTransportNotFound, transportName)
	}

	if err := sender.Send(ctx, env); err != nil {
		return DispatchResult{}, fmt.Errorf("messenger: transport %q send failed: %w", transportName, err)
	}

	return DispatchResult{Envelope: env, Async: true}, nil
}
