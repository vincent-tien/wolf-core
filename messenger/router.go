// router.go — Config-driven sync/async routing for the messenger bus.
//
// The Router maps message type names to transport names, determining whether
// a command runs synchronously (in-process handler) or asynchronously (sent
// to a named transport like "outbox" or "nats").
//
// Concurrency model: lock-free reads via atomic.Pointer to an immutable
// routeTable snapshot. Writes use copy-on-write under mutex. This means
// the hot path (Route) does zero locking — only 1 atomic load + 1 map
// lookup. Writes (AddRoute/UpdateRoutes) are rare (startup or config reload).
//
// An empty route table means "everything sync" — this is the default.
package messenger

import (
	"sync"
	"sync/atomic"
)

// routeTable is an immutable snapshot of message type → transport name mappings.
type routeTable struct {
	routes map[string]string
}

// Router determines which transport handles a given message type.
// Lock-free reads via atomic.Pointer, copy-on-write for updates.
//
// Route() returns "" for sync dispatch (no transport match).
type Router struct {
	table atomic.Pointer[routeTable]
	mu    sync.Mutex // protects write operations only
}

// NewRouter creates a router with empty routing table (everything sync).
func NewRouter() *Router {
	r := &Router{}
	r.table.Store(&routeTable{routes: make(map[string]string)})
	return r
}

// NewRouterFromMap creates a router with pre-configured routes.
func NewRouterFromMap(routes map[string]string) *Router {
	r := &Router{}
	cp := make(map[string]string, len(routes))
	for k, v := range routes {
		cp[k] = v
	}
	r.table.Store(&routeTable{routes: cp})
	return r
}

// Route returns the transport name for msg's type, or "" for sync dispatch.
// Hot path: 1 atomic load + 1 TypeNameOf + 1 map lookup. Zero locks.
func (r *Router) Route(msg any) string {
	t := r.table.Load()
	return t.routes[TypeNameOf(msg)]
}

// UpdateRoutes atomically replaces the entire routing table.
func (r *Router) UpdateRoutes(routes map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make(map[string]string, len(routes))
	for k, v := range routes {
		cp[k] = v
	}
	r.table.Store(&routeTable{routes: cp})
}

// AddRoute adds or updates a single route.
func (r *Router) AddRoute(msgType, transportName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.table.Load()
	cp := make(map[string]string, len(old.routes)+1)
	for k, v := range old.routes {
		cp[k] = v
	}
	cp[msgType] = transportName
	r.table.Store(&routeTable{routes: cp})
}

// RemoveRoute removes a single route.
func (r *Router) RemoveRoute(msgType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.table.Load()
	cp := make(map[string]string, len(old.routes))
	for k, v := range old.routes {
		if k != msgType {
			cp[k] = v
		}
	}
	r.table.Store(&routeTable{routes: cp})
}

// Routes returns a copy of the routing table (for debugging).
func (r *Router) Routes() map[string]string {
	t := r.table.Load()
	cp := make(map[string]string, len(t.routes))
	for k, v := range t.routes {
		cp[k] = v
	}
	return cp
}
