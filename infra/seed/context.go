// context.go — Seed execution context carrying DB, config, logger, and reference store.
package seed

import (
	"database/sql"

	"github.com/vincent-tien/wolf-core/infra/config"
	"go.uber.org/zap"
)

// SeedContext is the execution bag passed to every seeder. It provides access
// to database connections, logging, cross-seeder references, and runtime flags.
type SeedContext struct {
	db     *sql.DB
	tx     *sql.Tx
	logger *zap.Logger
	refs   *ReferenceStore
	env    string
	cfg    *config.Config
	dryRun bool
}

// SeedContextOption configures a SeedContext.
type SeedContextOption func(*SeedContext)

// NewSeedContext creates a SeedContext with the given database and options.
func NewSeedContext(db *sql.DB, opts ...SeedContextOption) *SeedContext {
	sc := &SeedContext{
		db:   db,
		refs: NewReferenceStore(),
	}
	for _, opt := range opts {
		opt(sc)
	}
	return sc
}

// WithTx sets the active transaction on the context.
func WithTx(tx *sql.Tx) SeedContextOption {
	return func(sc *SeedContext) { sc.tx = tx }
}

// WithLogger sets the logger.
func WithLogger(l *zap.Logger) SeedContextOption {
	return func(sc *SeedContext) { sc.logger = l }
}

// WithRefs sets a shared ReferenceStore.
func WithRefs(refs *ReferenceStore) SeedContextOption {
	return func(sc *SeedContext) { sc.refs = refs }
}

// WithEnv sets the environment name.
func WithEnv(env string) SeedContextOption {
	return func(sc *SeedContext) { sc.env = env }
}

// WithConfig sets the application config.
func WithConfig(cfg *config.Config) SeedContextOption {
	return func(sc *SeedContext) { sc.cfg = cfg }
}

// WithDryRun enables dry-run mode.
func WithDryRun(dryRun bool) SeedContextOption {
	return func(sc *SeedContext) { sc.dryRun = dryRun }
}

// DB returns the underlying database connection.
func (sc *SeedContext) DB() *sql.DB { return sc.db }

// Tx returns the active transaction, or nil if none.
func (sc *SeedContext) Tx() *sql.Tx { return sc.tx }

// DBTX returns the active transaction if set, otherwise the database connection.
// This value can be passed directly to sqlc-generated New() constructors.
func (sc *SeedContext) DBTX() DBTX {
	if sc.tx != nil {
		return sc.tx
	}
	return sc.db
}

// Logger returns the context logger. Returns a nop logger if none was set.
func (sc *SeedContext) Logger() *zap.Logger {
	if sc.logger == nil {
		return zap.NewNop()
	}
	return sc.logger
}

// Refs returns the shared reference store for cross-seeder data passing.
func (sc *SeedContext) Refs() *ReferenceStore { return sc.refs }

// Env returns the current environment name (e.g. "development", "staging").
func (sc *SeedContext) Env() string { return sc.env }

// Config returns the application configuration.
func (sc *SeedContext) Config() *config.Config { return sc.cfg }

// DryRun returns true if the runner is in preview mode.
func (sc *SeedContext) DryRun() bool { return sc.dryRun }

