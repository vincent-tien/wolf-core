// Package seed provides a database seeding framework inspired by Laravel's
// db:seed and Symfony's DoctrineFixturesBundle. Seeders are registered
// explicitly and executed in dependency order.
package seed

import (
	"context"
	"database/sql"
)

// Seeder is the minimal contract every seeder must implement.
type Seeder interface {
	// Name returns a unique dot-separated identifier (e.g. "iam.roles").
	Name() string
	// Groups returns the groups this seeder belongs to (e.g. ["core", "iam"]).
	Groups() []string
	// Environments returns which environments this seeder runs in.
	// Return nil or empty slice to run in all environments.
	Environments() []string
	// DependsOn returns names of seeders that must run before this one.
	DependsOn() []string
	// Seed performs the actual seeding work.
	Seed(ctx context.Context, sc *SeedContext) error
}

// DBTX matches the sqlc-generated DBTX interface, allowing seeders to work
// with both *sql.DB and *sql.Tx transparently.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ConditionalSeeder is an optional interface. Seeders that implement it can
// skip execution dynamically based on runtime conditions.
type ConditionalSeeder interface {
	ShouldRun(ctx context.Context, sc *SeedContext) bool
}

// TruncatingSeeder is an optional interface. Seeders that implement it declare
// which tables should be truncated during --fresh mode.
type TruncatingSeeder interface {
	TruncateTables() []string
}

// seedingKey is the context key for tagging a context as a seeding operation.
type seedingKey struct{}

// eventsDisabledKey is the context key for suppressing domain events.
type eventsDisabledKey struct{}

// WithSeeding tags the context as a seeding operation.
func WithSeeding(ctx context.Context) context.Context {
	return context.WithValue(ctx, seedingKey{}, true)
}

// IsSeeding returns true if the context was tagged by WithSeeding.
func IsSeeding(ctx context.Context) bool {
	v, _ := ctx.Value(seedingKey{}).(bool)
	return v
}

// WithEventsDisabled returns a context that signals event handlers to skip.
func WithEventsDisabled(ctx context.Context) context.Context {
	return context.WithValue(ctx, eventsDisabledKey{}, true)
}

// EventsDisabled returns true if domain events should be suppressed.
func EventsDisabled(ctx context.Context) bool {
	v, _ := ctx.Value(eventsDisabledKey{}).(bool)
	return v
}
