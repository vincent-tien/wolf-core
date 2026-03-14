// registry.go — Module instantiation registry with duplicate prevention and
// dependency-based boot ordering.
//
// Bootstrap receives a Manifest (slice of CatalogEntry) from the application,
// creates each module via its factory, and hands it to a Registry. The Registry
// deduplicates by name and — when any module implements runtime.DependencyDeclarer
// — performs topological sorting (Kahn's algorithm) to ensure correct startup order.
//
// Flow: Manifest() → factory(Container) → Registry.Register(module) →
// Registry.Modules() → bootstrap.RegisterModule() for each in sorted order.
package modular

import (
	"fmt"

	"github.com/vincent-tien/wolf-core/runtime"
)

// ModuleFactory is a constructor that receives the platform Container and
// returns a fully-wired runtime.Module.
type ModuleFactory func(c *Container) runtime.Module

// CatalogEntry pairs a module name with its factory. Modules export an Entry()
// function and the application's Manifest() collects them into an explicit slice.
type CatalogEntry struct {
	Name         string
	Factory      ModuleFactory
	SeedProvider any // optional: func(*seed.Registry), type-asserted by seed CLI
}

// Registry holds instantiated modules and provides duplicate prevention
// and dependency-based boot ordering.
type Registry struct {
	modules []runtime.Module
	seen    map[string]struct{}
}

// NewRegistry creates an empty module registry.
func NewRegistry() *Registry {
	return &Registry{
		seen: make(map[string]struct{}),
	}
}

// Register adds a module to the registry. Returns an error if a module
// with the same name has already been registered.
func (r *Registry) Register(m runtime.Module) error {
	name := m.Name()
	if _, exists := r.seen[name]; exists {
		return fmt.Errorf("modular: duplicate module %q", name)
	}
	r.seen[name] = struct{}{}
	r.modules = append(r.modules, m)
	return nil
}

// Modules returns all registered modules in boot order. If any module
// implements runtime.DependencyDeclarer, topological sorting is applied.
// Returns an error if a dependency cycle is detected or if a declared
// dependency is not registered.
func (r *Registry) Modules() ([]runtime.Module, error) {
	if !r.hasDependencies() {
		return r.modules, nil
	}
	return r.topologicalSort()
}

func (r *Registry) hasDependencies() bool {
	for _, m := range r.modules {
		if dd, ok := m.(runtime.DependencyDeclarer); ok && len(dd.DependsOn()) > 0 {
			return true
		}
	}
	return false
}

//nolint:gocognit,cyclop // topological sort is inherently complex
func (r *Registry) topologicalSort() ([]runtime.Module, error) {
	byName := make(map[string]runtime.Module, len(r.modules))
	for _, m := range r.modules {
		byName[m.Name()] = m
	}

	// Build adjacency: module -> modules it depends on.
	deps := make(map[string][]string, len(r.modules))
	for _, m := range r.modules {
		if dd, ok := m.(runtime.DependencyDeclarer); ok {
			for _, dep := range dd.DependsOn() {
				if _, exists := byName[dep]; !exists {
					return nil, fmt.Errorf("modular: module %q depends on unregistered module %q", m.Name(), dep)
				}
				deps[m.Name()] = append(deps[m.Name()], dep)
			}
		}
	}

	// Kahn's algorithm.
	inDegree := make(map[string]int, len(r.modules))
	for _, m := range r.modules {
		inDegree[m.Name()] = 0
	}
	for name, d := range deps {
		inDegree[name] = len(d)
		_ = d // used above
	}

	var queue []string
	for _, m := range r.modules {
		if inDegree[m.Name()] == 0 {
			queue = append(queue, m.Name())
		}
	}

	// Reverse adjacency for Kahn's: dep -> modules that depend on it.
	revDeps := make(map[string][]string, len(r.modules))
	for name, d := range deps {
		for _, dep := range d {
			revDeps[dep] = append(revDeps[dep], name)
		}
	}

	var sorted []runtime.Module
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		sorted = append(sorted, byName[name])

		for _, dependent := range revDeps[name] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(sorted) != len(r.modules) {
		return nil, fmt.Errorf("modular: dependency cycle detected among modules")
	}
	return sorted, nil
}
