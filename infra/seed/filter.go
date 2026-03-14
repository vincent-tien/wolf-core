// filter.go — Seeder filter pipeline (environment, groups, class, skip list).
package seed

import "slices"

// filterSeeders applies the full filter pipeline: environment, groups, class, skip.
func filterSeeders(seeders []Seeder, env string, opts RunOptions) []Seeder {
	seeders = filterByEnv(seeders, env)
	seeders = filterByGroups(seeders, opts.Groups)
	seeders = filterByClass(seeders, opts.Classes)
	seeders = filterBySkip(seeders, opts.Skip)
	return seeders
}

// filterByEnv keeps seeders whose Environments() list includes env, or whose
// list is nil/empty (meaning all environments).
func filterByEnv(seeders []Seeder, env string) []Seeder {
	if env == "" {
		return seeders
	}
	var out []Seeder
	for _, s := range seeders {
		envs := s.Environments()
		if len(envs) == 0 || slices.Contains(envs, env) {
			out = append(out, s)
		}
	}
	return out
}

// filterByGroups keeps seeders that belong to at least one of the given groups.
// If groups is empty, all seeders pass through.
func filterByGroups(seeders []Seeder, groups []string) []Seeder {
	if len(groups) == 0 {
		return seeders
	}
	groupSet := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		groupSet[g] = struct{}{}
	}
	var out []Seeder
	for _, s := range seeders {
		for _, sg := range s.Groups() {
			if _, ok := groupSet[sg]; ok {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// filterByClass keeps only seeders whose Name() is in the classes list.
// If classes is empty, all seeders pass through.
func filterByClass(seeders []Seeder, classes []string) []Seeder {
	if len(classes) == 0 {
		return seeders
	}
	classSet := make(map[string]struct{}, len(classes))
	for _, c := range classes {
		classSet[c] = struct{}{}
	}
	var out []Seeder
	for _, s := range seeders {
		if _, ok := classSet[s.Name()]; ok {
			out = append(out, s)
		}
	}
	return out
}

// filterBySkip removes seeders whose Name() is in the skip list.
func filterBySkip(seeders []Seeder, skip []string) []Seeder {
	if len(skip) == 0 {
		return seeders
	}
	skipSet := make(map[string]struct{}, len(skip))
	for _, s := range skip {
		skipSet[s] = struct{}{}
	}
	var out []Seeder
	for _, s := range seeders {
		if _, ok := skipSet[s.Name()]; !ok {
			out = append(out, s)
		}
	}
	return out
}
