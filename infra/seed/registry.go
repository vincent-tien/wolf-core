// registry.go — Seeder registration with explicit ordering (no init()-based catalog).
package seed

import "fmt"

// Registry holds an ordered list of seeders for explicit registration.
// Unlike the previous init()-based catalog, seeders are added manually
// via Register calls in a central RegisterAll function.
type Registry struct {
	seeders []Seeder
	seen    map[string]struct{}
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		seen: make(map[string]struct{}),
	}
}

// Register adds a seeder to the registry. Returns an error if a seeder
// with the same name is already registered.
func (r *Registry) Register(s Seeder) error {
	name := s.Name()
	if _, exists := r.seen[name]; exists {
		return fmt.Errorf("seed: duplicate seeder %q", name)
	}
	r.seen[name] = struct{}{}
	r.seeders = append(r.seeders, s)
	return nil
}

// MustRegister adds a seeder to the registry, panicking on duplicate names.
func (r *Registry) MustRegister(s Seeder) {
	if err := r.Register(s); err != nil {
		panic(err)
	}
}

// Seeders returns a copy of all registered seeders in registration order.
func (r *Registry) Seeders() []Seeder {
	out := make([]Seeder, len(r.seeders))
	copy(out, r.seeders)
	return out
}

// Names returns the names of all registered seeders in registration order.
func (r *Registry) Names() []string {
	names := make([]string, len(r.seeders))
	for i, s := range r.seeders {
		names[i] = s.Name()
	}
	return names
}
