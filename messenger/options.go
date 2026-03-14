// options.go — Functional options for MessageBus construction.
//
// Options are applied once during NewBus() and affect the immutable
// middleware chain, transport map, router, and handler registry.
// After construction, the bus is fully configured and cannot be reconfigured.
package messenger

// BusOption configures a MessageBus during construction.
type BusOption interface {
	apply(b *MessageBus, mws *[]Middleware)
}

type busOptionFunc struct {
	fn func(b *MessageBus, mws *[]Middleware)
}

func (o busOptionFunc) apply(b *MessageBus, mws *[]Middleware) { o.fn(b, mws) }

// WithMiddleware appends middleware to the bus dispatch chain.
// Order matters: first added = first executed (outermost).
func WithMiddleware(middleware ...Middleware) BusOption {
	return busOptionFunc{fn: func(_ *MessageBus, mws *[]Middleware) {
		*mws = append(*mws, middleware...)
	}}
}

// WithTransport registers a named transport sender.
func WithTransport(name string, sender Sender) BusOption {
	return busOptionFunc{fn: func(b *MessageBus, _ *[]Middleware) {
		b.transports[name] = sender
	}}
}

// WithRouter sets the message router.
func WithRouter(router *Router) BusOption {
	return busOptionFunc{fn: func(b *MessageBus, _ *[]Middleware) {
		b.router = router
	}}
}

// WithHandlerRegistry uses an existing registry instead of creating a new one.
func WithHandlerRegistry(reg *HandlerRegistry) BusOption {
	return busOptionFunc{fn: func(b *MessageBus, _ *[]Middleware) {
		b.handlers = reg
	}}
}
