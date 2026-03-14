// sort.go — Topological sort (Kahn's algorithm) for seeder dependency ordering.
package seed

import "fmt"

// topologicalSort orders seeders using Kahn's algorithm so that dependencies
// run before dependents. Returns an error on cycles or missing dependencies.
func topologicalSort(seeders []Seeder) ([]Seeder, error) {
	if len(seeders) == 0 {
		return nil, nil
	}

	byName := make(map[string]Seeder, len(seeders))
	for _, s := range seeders {
		byName[s.Name()] = s
	}

	deps, err := buildDeps(seeders, byName)
	if err != nil {
		return nil, err
	}

	return kahnSort(seeders, byName, deps)
}

// buildDeps validates and collects forward dependencies for each seeder.
func buildDeps(seeders []Seeder, byName map[string]Seeder) (map[string][]string, error) {
	deps := make(map[string][]string, len(seeders))
	for _, s := range seeders {
		for _, dep := range s.DependsOn() {
			if _, exists := byName[dep]; !exists {
				return nil, fmt.Errorf("seed: seeder %q depends on unregistered seeder %q", s.Name(), dep)
			}
			deps[s.Name()] = append(deps[s.Name()], dep)
		}
	}
	return deps, nil
}

// kahnSort performs Kahn's topological sort algorithm.
func kahnSort(seeders []Seeder, byName map[string]Seeder, deps map[string][]string) ([]Seeder, error) {
	inDegree := make(map[string]int, len(seeders))
	for _, s := range seeders {
		inDegree[s.Name()] = len(deps[s.Name()])
	}

	var queue []string
	for _, s := range seeders {
		if inDegree[s.Name()] == 0 {
			queue = append(queue, s.Name())
		}
	}

	// Reverse adjacency: dep -> seeders that depend on it.
	revDeps := make(map[string][]string, len(seeders))
	for name, d := range deps {
		for _, dep := range d {
			revDeps[dep] = append(revDeps[dep], name)
		}
	}

	var sorted []Seeder
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

	if len(sorted) != len(seeders) {
		return nil, fmt.Errorf("seed: dependency cycle detected among seeders")
	}
	return sorted, nil
}
