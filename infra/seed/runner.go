// runner.go — Executes seeders in topological order with dry-run and fresh modes.
package seed

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/vincent-tien/wolf-core/infra/config"
	"go.uber.org/zap"
)

// TxMode controls how transactions are managed during seeding.
type TxMode string

const (
	// TxModeGlobal wraps all seeders in a single transaction. A failure in
	// any seeder rolls back everything.
	TxModeGlobal TxMode = "global"
	// TxModePerSeeder wraps each seeder in its own transaction. Failures are
	// isolated — successful seeders are committed.
	TxModePerSeeder TxMode = "per-seeder"
)

// RunOptions controls which seeders run and how.
type RunOptions struct {
	Groups  []string // --group=core,demo
	Classes []string // --class=iam.roles
	Skip    []string // --skip=iam.demo_users
	Env     string   // --env=staging
	TxMode  TxMode   // --tx=global
	DryRun  bool     // --dry-run
	Fresh   bool     // --fresh (truncate seeded tables)
	Force   bool     // --force (required for production)
}

// SeederResult records the outcome of a single seeder execution.
type SeederResult struct {
	Name     string
	Status   string // "ok", "error", "dry-run", "conditional-skip"
	Duration time.Duration
	Error    error
}

// Runner orchestrates seeder execution with filtering, ordering, transactions,
// and result reporting.
type Runner struct {
	db       *sql.DB
	cfg      *config.Config
	logger   *zap.Logger
	registry *Registry
}

// NewRunner creates a Runner with the given database, config, logger, and registry.
func NewRunner(db *sql.DB, cfg *config.Config, logger *zap.Logger, registry *Registry) *Runner {
	return &Runner{
		db:       db,
		cfg:      cfg,
		logger:   logger,
		registry: registry,
	}
}

// Run executes seeders according to the given options.
func (r *Runner) Run(ctx context.Context, opts RunOptions) ([]SeederResult, error) {
	ctx = WithSeeding(ctx)
	ctx = WithEventsDisabled(ctx)

	env := r.resolveEnv(opts)
	if env == "production" && !opts.Force {
		return nil, fmt.Errorf("seed: refusing to run in production without --force")
	}

	seeders := filterSeeders(r.registry.Seeders(), env, opts)

	if opts.Fresh {
		if err := r.handleFresh(ctx, seeders); err != nil {
			return nil, err
		}
	}

	if len(seeders) == 0 {
		r.logger.Info("seed: no seeders matched filters")
		return nil, nil
	}

	sorted, err := topologicalSort(seeders)
	if err != nil {
		return nil, err
	}

	r.logger.Info("seed: starting",
		zap.String("env", env),
		zap.String("tx_mode", string(opts.TxMode)),
		zap.Bool("dry_run", opts.DryRun),
		zap.Int("seeders", len(sorted)),
	)

	var results []SeederResult
	switch opts.TxMode {
	case TxModePerSeeder:
		results, err = r.runPerSeeder(ctx, sorted, env, opts)
	default:
		results, err = r.runGlobal(ctx, sorted, env, opts)
	}
	if err != nil {
		return results, err
	}

	r.printSummary(results)
	return results, nil
}

func (r *Runner) resolveEnv(opts RunOptions) string {
	if opts.Env != "" {
		return opts.Env
	}
	return r.cfg.App.Env
}

func (r *Runner) newSeedContext(tx *sql.Tx, refs *ReferenceStore, env string, dryRun bool) *SeedContext {
	return NewSeedContext(r.db,
		WithTx(tx),
		WithLogger(r.logger),
		WithRefs(refs),
		WithEnv(env),
		WithConfig(r.cfg),
		WithDryRun(dryRun),
	)
}

func (r *Runner) handleFresh(ctx context.Context, seeders []Seeder) error {
	seen := make(map[string]struct{})
	var tables []string
	for _, s := range seeders {
		if ts, ok := s.(TruncatingSeeder); ok {
			for _, t := range ts.TruncateTables() {
				if _, exists := seen[t]; !exists {
					seen[t] = struct{}{}
					tables = append(tables, t)
				}
			}
		}
	}

	if len(tables) > 0 {
		r.logger.Info("seed: truncating tables", zap.Strings("tables", tables))
		if err := TruncateTables(ctx, r.db, tables); err != nil {
			return fmt.Errorf("seed: truncation failed: %w", err)
		}
	}

	return nil
}

func (r *Runner) runGlobal(
	ctx context.Context,
	seeders []Seeder,
	env string,
	opts RunOptions,
) ([]SeederResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("seed: failed to begin transaction: %w", err)
	}

	refs := NewReferenceStore()
	sc := r.newSeedContext(tx, refs, env, opts.DryRun)
	results := make([]SeederResult, 0, len(seeders))

	for _, s := range seeders {
		result := r.executeSingle(ctx, s, sc, opts.DryRun)
		results = append(results, result)

		if result.Error != nil {
			r.logger.Error("seed: rolling back all seeders",
				zap.String("failed_seeder", s.Name()),
				zap.Error(result.Error),
			)
			if rbErr := tx.Rollback(); rbErr != nil {
				r.logger.Error("seed: rollback failed", zap.Error(rbErr))
			}
			return results, result.Error
		}
	}

	if opts.DryRun {
		if err := tx.Rollback(); err != nil {
			return results, fmt.Errorf("seed: dry-run rollback failed: %w", err)
		}
		return results, nil
	}

	if err := tx.Commit(); err != nil {
		return results, fmt.Errorf("seed: commit failed: %w", err)
	}
	return results, nil
}

func (r *Runner) runPerSeeder(
	ctx context.Context,
	seeders []Seeder,
	env string,
	opts RunOptions,
) ([]SeederResult, error) {
	refs := NewReferenceStore()
	results := make([]SeederResult, 0, len(seeders))

	for _, s := range seeders {
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return results, fmt.Errorf("seed: failed to begin transaction for %s: %w", s.Name(), err)
		}

		sc := r.newSeedContext(tx, refs, env, opts.DryRun)
		result := r.executeSingle(ctx, s, sc, opts.DryRun)
		results = append(results, result)

		if result.Error != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				r.logger.Error("seed: rollback failed", zap.String("seeder", s.Name()), zap.Error(rbErr))
			}
			r.logger.Warn("seed: seeder failed, continuing", zap.String("seeder", s.Name()), zap.Error(result.Error))
			continue
		}

		if opts.DryRun {
			if err := tx.Rollback(); err != nil {
				return results, fmt.Errorf("seed: dry-run rollback failed for %s: %w", s.Name(), err)
			}
			continue
		}

		if err := tx.Commit(); err != nil {
			result.Error = fmt.Errorf("commit: %w", err)
			result.Status = "error"
			r.logger.Error("seed: commit failed", zap.String("seeder", s.Name()), zap.Error(err))
		}
	}

	return results, nil
}

func (r *Runner) executeSingle(ctx context.Context, s Seeder, sc *SeedContext, dryRun bool) SeederResult {
	name := s.Name()
	start := time.Now()

	if cs, ok := s.(ConditionalSeeder); ok {
		if !cs.ShouldRun(ctx, sc) {
			return SeederResult{Name: name, Status: "conditional-skip", Duration: time.Since(start)}
		}
	}

	if dryRun {
		r.logger.Info("seed: [dry-run] would execute", zap.String("seeder", name))
		return SeederResult{Name: name, Status: "dry-run", Duration: time.Since(start)}
	}

	r.logger.Info("seed: executing", zap.String("seeder", name))
	if err := s.Seed(ctx, sc); err != nil {
		return SeederResult{Name: name, Status: "error", Duration: time.Since(start), Error: err}
	}
	elapsed := time.Since(start)

	r.logger.Info("seed: completed", zap.String("seeder", name), zap.Duration("duration", elapsed))
	return SeederResult{Name: name, Status: "ok", Duration: elapsed}
}

func (r *Runner) printSummary(results []SeederResult) {
	r.logger.Info("seed: --- summary ---")
	for _, res := range results {
		fields := []zap.Field{
			zap.String("seeder", res.Name),
			zap.String("status", res.Status),
			zap.Duration("duration", res.Duration),
		}
		if res.Error != nil {
			fields = append(fields, zap.Error(res.Error))
		}
		r.logger.Info("seed: result", fields...)
	}

	var ok, skipped, errored int
	for _, res := range results {
		switch res.Status {
		case "ok":
			ok++
		case "conditional-skip", "dry-run":
			skipped++
		case "error":
			errored++
		}
	}

	totals := []zap.Field{
		zap.Int("ok", ok),
		zap.Int("skipped", skipped),
		zap.Int("errors", errored),
		zap.Int("total", len(results)),
	}
	if errored > 0 {
		r.logger.Warn("seed: completed with errors", totals...)
	} else {
		r.logger.Info("seed: totals", totals...)
	}
}

// ParseCSV splits a comma-separated string into a trimmed slice.
// Empty input returns nil.
func ParseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
