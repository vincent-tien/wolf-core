// factory.go — Transport factory registry for DSN-based transport creation.
package transport

import "fmt"

// Factory creates transports from DSN strings.
type Factory interface {
	Supports(dsn string) bool
	Create(dsn string, opts map[string]any) (Transport, error)
}

// FactoryRegistry finds the right factory for a given DSN.
type FactoryRegistry struct {
	factories []Factory
}

// NewFactoryRegistry creates an empty factory registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{}
}

// Register adds a factory to the registry.
func (r *FactoryRegistry) Register(f Factory) {
	r.factories = append(r.factories, f)
}

// Create finds the first factory that supports the DSN and creates a transport.
func (r *FactoryRegistry) Create(dsn string, opts map[string]any) (Transport, error) {
	for _, f := range r.factories {
		if f.Supports(dsn) {
			return f.Create(dsn, opts)
		}
	}
	return nil, fmt.Errorf("transport: no factory supports DSN %q", dsn)
}
