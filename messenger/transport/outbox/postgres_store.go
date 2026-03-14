// postgres_store.go — PostgreSQL implementation of the outbox transport Store.
package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vincent-tien/wolf-core/tx"
)

// DefaultMaxRetries is the default number of retries before an entry is
// excluded from GetUnpublished. Entries exceeding this need manual
// inspection or DLQ processing.
const DefaultMaxRetries = 10

// PostgresStore implements Store against the outbox_events table.
type PostgresStore struct {
	db           *sql.DB
	claimTimeout time.Duration
	maxRetries   int
}

// StoreOption configures PostgresStore.
type StoreOption func(*PostgresStore)

// WithClaimTimeout sets how long a claimed entry stays locked before another
// worker can reclaim it. Default: 5 minutes. Set to 0 to disable claiming
// (plain SELECT, single-worker mode).
func WithClaimTimeout(d time.Duration) StoreOption {
	return func(s *PostgresStore) { s.claimTimeout = d }
}

// WithMaxRetries sets the retry threshold. Entries with retry_count >= n
// are excluded from GetUnpublished. Default: 10.
func WithMaxRetries(n int) StoreOption {
	return func(s *PostgresStore) { s.maxRetries = n }
}

// NewPostgresStore creates a store backed by the given connection pool.
func NewPostgresStore(db *sql.DB, opts ...StoreOption) *PostgresStore {
	s := &PostgresStore{db: db, claimTimeout: 5 * time.Minute, maxRetries: DefaultMaxRetries}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *PostgresStore) executor(ctx context.Context) execer {
	if raw, ok := tx.Extract(ctx); ok {
		if sqlTx, ok := raw.(*sql.Tx); ok {
			return sqlTx
		}
	}
	return s.db
}

// Insert writes an entry into outbox_events. Transaction-aware via tx.Extract.
func (s *PostgresStore) Insert(ctx context.Context, entry Entry) error {
	const query = `
		INSERT INTO outbox_events
			(id, aggregate_type, aggregate_id, event_type, payload, trace_id, created_at, retry_count, last_error)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, 0, '')`

	_, err := s.executor(ctx).ExecContext(ctx, query,
		entry.ID, entry.AggregateType, entry.AggregateID,
		entry.EventType, entry.Payload, entry.TraceID, entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("outbox postgres: insert %q: %w", entry.ID, err)
	}
	return nil
}

// GetUnpublished returns up to batchSize unpublished entries ordered by creation time.
// Entries exceeding maxRetries (10) are excluded — they need manual inspection or DLQ.
//
// When claimTimeout > 0 (default 5 min), uses an atomic UPDATE...RETURNING with
// FOR UPDATE SKIP LOCKED so multiple relay workers can poll concurrently without
// processing the same entries. Stale claims (worker crash) are reclaimed after
// the timeout expires.
//
// When claimTimeout == 0, uses a plain SELECT (single-worker mode).
func (s *PostgresStore) GetUnpublished(ctx context.Context, batchSize int) ([]Entry, error) {
	if s.claimTimeout <= 0 {
		return s.getUnpublishedSimple(ctx, batchSize)
	}
	return s.getUnpublishedClaim(ctx, batchSize)
}

func (s *PostgresStore) getUnpublishedSimple(ctx context.Context, batchSize int) ([]Entry, error) {
	const query = `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, trace_id,
		       created_at, retry_count
		FROM   outbox_events
		WHERE  published_at IS NULL
		  AND  retry_count < $2
		ORDER  BY created_at ASC
		LIMIT  $1`

	return s.scanEntries(ctx, query, batchSize, s.maxRetries)
}

func (s *PostgresStore) getUnpublishedClaim(ctx context.Context, batchSize int) ([]Entry, error) {
	const query = `
		UPDATE outbox_events
		SET    claimed_at = NOW()
		WHERE  id IN (
		    SELECT id FROM outbox_events
		    WHERE  published_at IS NULL
		      AND  retry_count < $2
		      AND  (claimed_at IS NULL OR claimed_at < NOW() - make_interval(secs => $3))
		    ORDER  BY created_at ASC
		    LIMIT  $1
		    FOR UPDATE SKIP LOCKED
		)
		RETURNING id, aggregate_type, aggregate_id, event_type, payload, trace_id,
		          created_at, retry_count`

	claimTimeoutSecs := int(s.claimTimeout.Seconds())
	return s.scanEntries(ctx, query, batchSize, s.maxRetries, claimTimeoutSecs)
}

func (s *PostgresStore) scanEntries(ctx context.Context, query string, args ...any) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("outbox postgres: query unpublished: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(
			&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType,
			&e.Payload, &e.TraceID, &e.CreatedAt, &e.RetryCount,
		); err != nil {
			return nil, fmt.Errorf("outbox postgres: scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox postgres: rows: %w", err)
	}
	return entries, nil
}

// MarkPublished sets published_at and clears claimed_at for the given entry ID.
func (s *PostgresStore) MarkPublished(ctx context.Context, id string) error {
	const query = `UPDATE outbox_events SET published_at = NOW(), claimed_at = NULL WHERE id = $1`
	if _, err := s.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("outbox postgres: mark published %q: %w", id, err)
	}
	return nil
}

// IncrementRetry bumps retry_count, records the last error, and clears claimed_at
// so the entry is immediately available for the next poll cycle.
func (s *PostgresStore) IncrementRetry(ctx context.Context, id string, lastError string) error {
	const query = `
		UPDATE outbox_events
		SET    retry_count = retry_count + 1, last_error = $2, claimed_at = NULL
		WHERE  id = $1`
	if _, err := s.db.ExecContext(ctx, query, id, lastError); err != nil {
		return fmt.Errorf("outbox postgres: increment retry %q: %w", id, err)
	}
	return nil
}
