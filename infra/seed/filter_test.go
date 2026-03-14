package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterByEnv(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "all-envs", envs: nil},
		&stubSeeder{name: "dev-only", envs: []string{"development"}},
		&stubSeeder{name: "staging-only", envs: []string{"staging"}},
		&stubSeeder{name: "dev-staging", envs: []string{"development", "staging"}},
	}

	tests := []struct {
		name string
		env  string
		want []string
	}{
		{"empty env passes all", "", []string{"all-envs", "dev-only", "staging-only", "dev-staging"}},
		{"development", "development", []string{"all-envs", "dev-only", "dev-staging"}},
		{"staging", "staging", []string{"all-envs", "staging-only", "dev-staging"}},
		{"production", "production", []string{"all-envs"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterByEnv(seeders, tt.env)
			assert.Equal(t, tt.want, seederNames(filtered))
		})
	}
}

func TestFilterByGroups(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "a", groups: []string{"core"}},
		&stubSeeder{name: "b", groups: []string{"demo"}},
		&stubSeeder{name: "c", groups: []string{"core", "demo"}},
		&stubSeeder{name: "d", groups: []string{"test"}},
	}

	tests := []struct {
		name   string
		groups []string
		want   []string
	}{
		{"empty groups passes all", nil, []string{"a", "b", "c", "d"}},
		{"core only", []string{"core"}, []string{"a", "c"}},
		{"demo only", []string{"demo"}, []string{"b", "c"}},
		{"core and demo", []string{"core", "demo"}, []string{"a", "b", "c"}},
		{"no match", []string{"nonexistent"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterByGroups(seeders, tt.groups)
			names := seederNames(filtered)
			if tt.want == nil {
				assert.Empty(t, names)
			} else {
				assert.Equal(t, tt.want, names)
			}
		})
	}
}

func TestFilterByClass(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "iam.roles"},
		&stubSeeder{name: "iam.admin"},
		&stubSeeder{name: "product.demo"},
	}

	tests := []struct {
		name    string
		classes []string
		want    []string
	}{
		{"empty passes all", nil, []string{"iam.roles", "iam.admin", "product.demo"}},
		{"specific class", []string{"iam.roles"}, []string{"iam.roles"}},
		{"multiple classes", []string{"iam.roles", "product.demo"}, []string{"iam.roles", "product.demo"}},
		{"no match", []string{"nonexistent"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterByClass(seeders, tt.classes)
			names := seederNames(filtered)
			if tt.want == nil {
				assert.Empty(t, names)
			} else {
				assert.Equal(t, tt.want, names)
			}
		})
	}
}

func TestFilterBySkip(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "a"},
		&stubSeeder{name: "b"},
		&stubSeeder{name: "c"},
	}

	tests := []struct {
		name string
		skip []string
		want []string
	}{
		{"empty skip passes all", nil, []string{"a", "b", "c"}},
		{"skip one", []string{"b"}, []string{"a", "c"}},
		{"skip multiple", []string{"a", "c"}, []string{"b"}},
		{"skip all", []string{"a", "b", "c"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterBySkip(seeders, tt.skip)
			names := seederNames(filtered)
			if tt.want == nil {
				assert.Empty(t, names)
			} else {
				assert.Equal(t, tt.want, names)
			}
		})
	}
}

func TestFilterSeeders_Pipeline(t *testing.T) {
	seeders := []Seeder{
		&stubSeeder{name: "core.a", groups: []string{"core"}, envs: nil},
		&stubSeeder{name: "demo.b", groups: []string{"demo"}, envs: []string{"development"}},
		&stubSeeder{name: "demo.c", groups: []string{"demo"}, envs: []string{"development"}},
		&stubSeeder{name: "test.d", groups: []string{"test"}, envs: []string{"development"}},
	}

	opts := RunOptions{Groups: []string{"demo"}, Skip: []string{"demo.c"}}
	result := filterSeeders(seeders, "development", opts)
	assert.Equal(t, []string{"demo.b"}, seederNames(result))
}

func seederNames(seeders []Seeder) []string {
	if len(seeders) == 0 {
		return nil
	}
	names := make([]string, len(seeders))
	for i, s := range seeders {
		names[i] = s.Name()
	}
	return names
}
