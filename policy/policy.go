// Package policy provides a Strategy pattern for domain policies.
// Policies encapsulate business decisions that may vary by context
// (e.g. pricing, discount, shipping). New strategies are added as new
// structs — existing code remains untouched (OCP).
package policy

import "fmt"

// Policy defines a named business strategy that transforms input I into output O.
type Policy[I, O any] interface {
	Name() string
	Apply(input I) (O, error)
}

// Func creates a Policy from a name and function.
func Func[I, O any](name string, fn func(I) (O, error)) Policy[I, O] {
	return &funcPolicy[I, O]{name: name, fn: fn}
}

type funcPolicy[I, O any] struct {
	name string
	fn   func(I) (O, error)
}

func (p *funcPolicy[I, O]) Name() string            { return p.name }
func (p *funcPolicy[I, O]) Apply(in I) (O, error)   { return p.fn(in) }

// ApplyFirst tries each policy in order and returns the first successful result.
// Returns an error if all policies fail.
func ApplyFirst[I, O any](input I, policies ...Policy[I, O]) (O, error) {
	var zero O
	for _, p := range policies {
		result, err := p.Apply(input)
		if err == nil {
			return result, nil
		}
	}
	return zero, fmt.Errorf("policy: all %d policies failed", len(policies))
}
