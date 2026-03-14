package seed

import "context"

// stubSeeder is a minimal Seeder implementation for testing.
type stubSeeder struct {
	name        string
	groups      []string
	envs        []string
	deps        []string
	seedFn      func(ctx context.Context, sc *SeedContext) error
	shouldRunFn func(ctx context.Context, sc *SeedContext) bool
	truncateFn  func() []string
}

func (s *stubSeeder) Name() string          { return s.name }
func (s *stubSeeder) Groups() []string       { return s.groups }
func (s *stubSeeder) Environments() []string { return s.envs }
func (s *stubSeeder) DependsOn() []string    { return s.deps }

func (s *stubSeeder) Seed(ctx context.Context, sc *SeedContext) error {
	if s.seedFn != nil {
		return s.seedFn(ctx, sc)
	}
	return nil
}

// Optional interfaces — only implement if the field is set.

func (s *stubSeeder) ShouldRun(ctx context.Context, sc *SeedContext) bool {
	if s.shouldRunFn != nil {
		return s.shouldRunFn(ctx, sc)
	}
	return true
}

func (s *stubSeeder) TruncateTables() []string {
	if s.truncateFn != nil {
		return s.truncateFn()
	}
	return nil
}
