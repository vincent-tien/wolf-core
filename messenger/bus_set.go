// bus_set.go — Named bus collection for separating command and query pipelines.
//
// BusSet allows wiring separate MessageBus instances for commands vs queries,
// each with its own middleware chain, handler registry, and routing table.
// This enables different middleware stacks (e.g., commands get outbox
// middleware, queries get caching middleware) while keeping a unified API.
//
// Typical setup in bootstrap:
//
//	set := messenger.NewBusSet(
//	    messenger.NamedBus{Name: "command", Bus: cmdBus},
//	    messenger.NamedBus{Name: "query",   Bus: queryBus},
//	)
//	set.Command().Dispatch(ctx, createUser)
//	set.Query().Query(ctx, getUserByID)
package messenger

import (
	"fmt"
	"maps"
	"slices"
)

// Well-known bus names for BusSet convenience methods.
const (
	BusNameCommand = "command"
	BusNameQuery   = "query"
)

// NamedBus pairs a name with a MessageBus for BusSet construction.
type NamedBus struct {
	Name string
	Bus  *MessageBus
}

// BusSet manages multiple named MessageBus instances.
// Each bus has its own independent registry, middleware chain, and router.
// Immutable after construction.
type BusSet struct {
	buses map[string]*MessageBus
	names []string
}

// NewBusSet creates a BusSet from named buses.
// Panics on duplicate names (programming error at startup).
func NewBusSet(buses ...NamedBus) *BusSet {
	m := make(map[string]*MessageBus, len(buses))
	for _, nb := range buses {
		if _, exists := m[nb.Name]; exists {
			panic(fmt.Sprintf("messenger: duplicate bus name %q in BusSet", nb.Name))
		}
		m[nb.Name] = nb.Bus
	}
	return &BusSet{
		buses: m,
		names: slices.Sorted(maps.Keys(m)),
	}
}

// Bus returns the bus with the given name. Panics if not found.
func (s *BusSet) Bus(name string) *MessageBus {
	b, ok := s.buses[name]
	if !ok {
		panic(fmt.Sprintf("messenger: bus %q not found in BusSet", name))
	}
	return b
}

// Get returns the bus with the given name and a boolean indicating existence.
func (s *BusSet) Get(name string) (*MessageBus, bool) {
	b, ok := s.buses[name]
	return b, ok
}

// Command is a convenience shortcut for Bus(BusNameCommand).
func (s *BusSet) Command() *MessageBus { return s.Bus(BusNameCommand) }

// Query is a convenience shortcut for Bus(BusNameQuery).
func (s *BusSet) Query() *MessageBus { return s.Bus(BusNameQuery) }

// Names returns all registered bus names in sorted order.
func (s *BusSet) Names() []string { return s.names }
