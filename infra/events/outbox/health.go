// health.go — Readiness probe that detects outbox relay lag.
package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// LagChecker provides a readiness probe that detects when the outbox relay
// falls behind. It queries the count of unpublished entries older than a
// configurable threshold. If any such entries exist, the check fails.
type LagChecker struct {
	db        *sql.DB
	threshold time.Duration
}

// NewLagChecker constructs a LagChecker. threshold is the maximum age an
// unpublished outbox entry may have before the check reports unhealthy.
func NewLagChecker(db *sql.DB, threshold time.Duration) *LagChecker {
	return &LagChecker{db: db, threshold: threshold}
}

// HealthCheck returns nil when no unpublished entries exceed the threshold
// age, or an error describing the lag.
func (c *LagChecker) HealthCheck(ctx context.Context) error {
	const query = `
		SELECT COUNT(*)
		FROM   outbox_events
		WHERE  published_at IS NULL
		AND    created_at < $1`

	cutoff := time.Now().UTC().Add(-c.threshold)

	var count int64
	if err := c.db.QueryRowContext(ctx, query, cutoff).Scan(&count); err != nil {
		return fmt.Errorf("outbox_lag: query: %w", err)
	}

	if count > 0 {
		return fmt.Errorf("outbox_lag: %d entries older than %s remain unpublished", count, c.threshold)
	}

	return nil
}
