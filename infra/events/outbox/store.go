// Package outbox implements the transactional outbox store for reliable
// at-least-once event delivery. Events are written to the outbox table inside
// the same database transaction as the domain state change, guaranteeing that
// an event is never lost even when the process crashes between DB write and
// broker publish.
package outbox

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/observability/logging"
	sharedErrors "github.com/vincent-tien/wolf-core/errors"
	"github.com/vincent-tien/wolf-core/event"
	"github.com/vincent-tien/wolf-core/tx"
)

// leaseTimeout is how long a claimed entry stays locked before becoming
// eligible for re-claim by another worker instance. 5 minutes is generous
// enough to survive slow broker round-trips while limiting the window of
// event delivery delay after a crash.
const leaseTimeout = 5 * time.Minute

// leaseTimeoutStr is pre-computed to avoid per-poll string allocation.
var leaseTimeoutStr = leaseTimeout.String()

// OutboxEntry represents a single row in the outbox_events table.
type OutboxEntry struct {
	// ID is the unique identifier for this outbox record (mirrors the event ID).
	ID string
	// AggregateType is the name of the aggregate that emitted the event.
	AggregateType string
	// AggregateID is the identifier of the aggregate instance.
	AggregateID string
	// EventType is the fully-qualified event type name (e.g. "order.created").
	EventType string
	// Payload is the JSON-encoded event body.
	Payload []byte
	// TraceID is the distributed trace identifier propagated from the request.
	TraceID string
	// Metadata carries the full observability context (correlation, causation, source).
	Metadata event.Metadata
	// CreatedAt is the time the outbox record was inserted.
	CreatedAt time.Time
	// ClaimedAt is set when a worker claims the entry for publishing.
	ClaimedAt *time.Time
	// LeaseToken prevents ABA re-claim conflicts between workers.
	LeaseToken string
	// PublishedAt is set when the relay successfully forwards the event to the broker.
	PublishedAt *time.Time
	// RetryCount tracks the number of failed delivery attempts.
	RetryCount int
	// LastError stores the most recent delivery error message.
	LastError string
}

// Store provides CRUD operations on the outbox_events table.
// All mutation methods that participate in domain transactions accept a *sql.Tx
// so they can be composed with aggregate persistence in a single atomic unit.
type Store struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewStore creates a *Store backed by the provided connection pool.
func NewStore(db *sql.DB, logger *zap.Logger) *Store {
	return &Store{db: db, logger: logger}
}

// execer abstracts *sql.DB and *sql.Tx for the Insert helper.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Insert writes a new outbox record. It extracts the transaction from ctx
// (injected by tx.Inject); if no transaction is present it falls back to
// the store's connection pool for non-transactional callers.
//
// Metadata is automatically enriched from the request context: OTel trace ID
// for distributed tracing correlation, and request ID as correlation ID for
// log-to-event linking. Callers may pass evt.GetMetadata() directly.
//
// When used with UnitOfWork the transaction is always present, guaranteeing
// atomicity between the domain write and the outbox insertion.
func (s *Store) Insert(ctx context.Context, evt event.Event, meta event.Metadata) error {
	enrichMetadataFromContext(ctx, &meta)
	payload, err := json.Marshal(evt.Payload())
	if err != nil {
		return fmt.Errorf("outbox: marshal event %q: %w", evt.EventID(), err)
	}

	metadataJSON, err := marshalMetadata(meta)
	if err != nil {
		return fmt.Errorf("outbox: event %q: %w", evt.EventID(), err)
	}

	const query = `
		INSERT INTO outbox_events
			(id, aggregate_type, aggregate_id, event_type, payload, trace_id, metadata, created_at, retry_count, last_error)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, 0, '')`

	db := s.executor(ctx)
	// Cast []byte to string for JSONB columns — pgx/stdlib sends []byte as
	// bytea which Postgres rejects for JSONB. string is sent as text which works.
	_, err = db.ExecContext(ctx, query,
		evt.EventID(),
		evt.AggregateType(),
		evt.AggregateID(),
		evt.EventType(),
		string(payload),
		meta.TraceID,
		jsonBytesToString(metadataJSON),
		evt.OccurredAt(),
	)
	if err != nil {
		return fmt.Errorf("outbox: insert event %q: %w", evt.EventID(), err)
	}

	return nil
}

// enrichMetadataFromContext fills empty trace/correlation fields from ctx.
func enrichMetadataFromContext(ctx context.Context, meta *event.Metadata) {
	if meta.TraceID == "" {
		meta.TraceID = logging.OTelTraceIDFromContext(ctx)
	}
	if meta.CorrelationID == "" {
		meta.CorrelationID = logging.RequestIDFromContext(ctx)
	}
}

// marshalMetadata serializes metadata to JSON. Returns nil when all fields
// are empty (the DB column is nullable JSONB, so nil means no metadata).
func marshalMetadata(meta event.Metadata) ([]byte, error) {
	if meta.IsZero() {
		return nil, nil
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return b, nil
}

// jsonBytesToString converts []byte to string for JSONB columns.
// Returns "{}" when input is nil (safe for NOT NULL JSONB columns).
// pgx/stdlib sends []byte as bytea — string is sent as text which Postgres accepts for JSONB.
func jsonBytesToString(b []byte) string {
	if b == nil {
		return "{}"
	}
	return string(b)
}

func (s *Store) executor(ctx context.Context) execer {
	if raw, ok := tx.Extract(ctx); ok {
		if sqlTx, ok := raw.(*sql.Tx); ok {
			return sqlTx
		}
	}
	return s.db
}

// ClaimBatch atomically selects up to batchSize unpublished outbox records and
// marks them as claimed (NOT published) inside the same transaction. The worker
// must call MarkPublished after successful broker delivery, or ReleaseClaim on
// failure.
//
// Rows are eligible for claiming when they are either:
//   - never claimed (claimed_at IS NULL, published_at IS NULL), or
//   - stale-claimed (claimed_at older than leaseTimeout, published_at IS NULL),
//     indicating a previous worker crashed before completing delivery.
//
// Consumer-side deduplication (inbox) remains the ultimate safety net for
// at-least-once delivery.
func (s *Store) ClaimBatch(ctx context.Context, batchSize int) (_ []OutboxEntry, err error) {
	token := generateLeaseToken()

	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("outbox: begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = dbTx.Rollback()
		}
	}()

	const selectQuery = `
		SELECT id, aggregate_type, aggregate_id, event_type, payload, trace_id, metadata,
		       created_at, claimed_at, lease_token, published_at, retry_count, last_error
		FROM   outbox_events
		WHERE  published_at IS NULL
		AND    (claimed_at IS NULL OR claimed_at < NOW() - $2::interval)
		ORDER  BY created_at ASC
		LIMIT  $1
		FOR UPDATE SKIP LOCKED`

	rows, err := dbTx.QueryContext(ctx, selectQuery, batchSize, leaseTimeoutStr)
	if err != nil {
		return nil, fmt.Errorf("outbox: query unpublished: %w", err)
	}
	defer sharedErrors.Do(&err, rows.Close, "close rows")

	entries := make([]OutboxEntry, 0, batchSize)
	for rows.Next() {
		var e OutboxEntry
		var metadataJSON []byte
		if err := rows.Scan(
			&e.ID, &e.AggregateType, &e.AggregateID, &e.EventType,
			&e.Payload, &e.TraceID, &metadataJSON,
			&e.CreatedAt, &e.ClaimedAt, &e.LeaseToken, &e.PublishedAt, &e.RetryCount, &e.LastError,
		); err != nil {
			return nil, fmt.Errorf("outbox: scan row: %w", err)
		}
		if len(metadataJSON) > 0 {
			if uerr := json.Unmarshal(metadataJSON, &e.Metadata); uerr != nil {
				s.logger.Debug("outbox: unmarshal metadata",
					zap.String("entry_id", e.ID),
					zap.Error(uerr),
				)
			}
		}
		if e.Metadata.TraceID == "" && e.TraceID != "" {
			e.Metadata.TraceID = e.TraceID
		}
		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox: iterate rows: %w", err)
	}

	if len(entries) > 0 {
		ids := make([]string, len(entries))
		for i := range entries {
			ids[i] = entries[i].ID
		}
		if err := claimEntriesTx(ctx, dbTx, ids, token); err != nil {
			return nil, fmt.Errorf("outbox: claim entries: %w", err)
		}
		for i := range entries {
			entries[i].LeaseToken = token
		}
	}

	if err := dbTx.Commit(); err != nil {
		return nil, fmt.Errorf("outbox: commit tx: %w", err)
	}

	return entries, nil
}

// ReleaseEntry pairs an outbox record ID with the error that caused the
// publish failure. Used by ReleaseClaims for batch release.
type ReleaseEntry struct {
	ID        string
	LastError string
}

// ReleaseClaim returns a previously claimed entry to the unclaimed pool by
// clearing claimed_at/lease_token and incrementing retry_count. Called by the
// worker when a publish attempt fails so the entry can be retried.
func (s *Store) ReleaseClaim(ctx context.Context, id string, lastError string) error {
	const query = `
		UPDATE outbox_events
		SET    claimed_at  = NULL,
		       lease_token = NULL,
		       retry_count = retry_count + 1,
		       last_error  = $2
		WHERE  id = $1`

	if _, err := s.db.ExecContext(ctx, query, id, lastError); err != nil {
		return fmt.Errorf("outbox: release claim %q: %w", id, err)
	}

	return nil
}

// ReleaseClaims batch-releases multiple claimed entries in a single SQL
// round-trip. Each entry gets its own last_error value via a VALUES join.
func (s *Store) ReleaseClaims(ctx context.Context, entries []ReleaseEntry) error {
	if len(entries) == 0 {
		return nil
	}

	if len(entries) == 1 {
		return s.ReleaseClaim(ctx, entries[0].ID, entries[0].LastError)
	}

	valuePlaceholders := make([]string, len(entries))
	args := make([]any, 0, len(entries)*2)
	for i, e := range entries {
		valuePlaceholders[i] = fmt.Sprintf("($%d::text, $%d::text)", i*2+1, i*2+2)
		args = append(args, e.ID, e.LastError)
	}

	query := fmt.Sprintf(`
		UPDATE outbox_events AS o
		SET    claimed_at  = NULL,
		       lease_token = NULL,
		       retry_count = retry_count + 1,
		       last_error  = v.last_error
		FROM   (VALUES %s) AS v(id, last_error)
		WHERE  o.id = v.id`,
		strings.Join(valuePlaceholders, ","),
	)

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("outbox: batch release claims: %w", err)
	}

	return nil
}

// MarkPublished sets published_at on entries that were successfully delivered
// to the broker. Called AFTER broker acknowledgement to guarantee durability.
// The leaseToken parameter prevents ABA conflicts: only the worker that
// claimed the entries can mark them as published.
func (s *Store) MarkPublished(ctx context.Context, ids []string, leaseToken string) error {
	if len(ids) == 0 {
		return nil
	}

	inClause, args := buildINClause(leaseToken, ids)
	query := fmt.Sprintf(
		`UPDATE outbox_events SET published_at = NOW() WHERE lease_token = $1 AND id IN (%s)`,
		inClause,
	)

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("outbox: mark published: %w", err)
	}

	return nil
}

// claimEntriesTx sets claimed_at and lease_token inside an existing transaction.
func claimEntriesTx(ctx context.Context, dbTx *sql.Tx, ids []string, token string) error {
	inClause, args := buildINClause(token, ids)
	query := fmt.Sprintf(
		`UPDATE outbox_events SET claimed_at = NOW(), lease_token = $1 WHERE id IN (%s)`,
		inClause,
	)

	if _, err := dbTx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("outbox: claim entries: %w", err)
	}

	return nil
}

// buildINClause builds a parameterised IN clause with a leading argument at $1.
// Returns the comma-separated placeholder string and the combined args slice.
func buildINClause(leadArg any, ids []string) (string, []any) {
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids)+1)
	args[0] = leadArg
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}
	return strings.Join(placeholders, ","), args
}

// generateLeaseToken creates a cryptographically random token for claim
// ownership. 16 bytes (128 bits) provides sufficient uniqueness.
func generateLeaseToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// InsertEntry writes a pre-built OutboxEntry to the outbox_events table using
// the store's connection pool. This is used by OutboxMiddleware which builds
// the entry from a messaging.RawMessage rather than from a domain event.
func (s *Store) InsertEntry(ctx context.Context, entry OutboxEntry) error {
	metadataJSON, err := marshalMetadata(entry.Metadata)
	if err != nil {
		return fmt.Errorf("outbox: entry %q: %w", entry.ID, err)
	}

	const query = `
		INSERT INTO outbox_events
			(id, aggregate_type, aggregate_id, event_type, payload, trace_id, metadata, created_at, retry_count, last_error)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, NOW(), 0, '')`

	if _, execErr := s.db.ExecContext(ctx, query,
		entry.ID,
		entry.AggregateType,
		entry.AggregateID,
		entry.EventType,
		string(entry.Payload),
		entry.TraceID,
		jsonBytesToString(metadataJSON),
	); execErr != nil {
		return fmt.Errorf("outbox: insert entry %q: %w", entry.ID, execErr)
	}

	return nil
}


// cleanupBatchLimit caps the number of rows deleted per Cleanup call to prevent
// long-running DELETEs from spiking DB latency. Callers should loop until
// deleted < limit to drain all stale entries.
const cleanupBatchLimit = 10_000

// Cleanup deletes up to cleanupBatchLimit outbox records whose published_at
// timestamp is older than retention. Returns the number of rows deleted.
// Call in a loop until deleted < cleanupBatchLimit to drain all stale entries.
func (s *Store) Cleanup(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-retention)

	const query = `
		DELETE FROM outbox_events
		WHERE  id IN (
			SELECT id FROM outbox_events
			WHERE  published_at IS NOT NULL
			AND    published_at < $1
			LIMIT  $2
		)`

	result, err := s.db.ExecContext(ctx, query, cutoff, cleanupBatchLimit)
	if err != nil {
		return 0, fmt.Errorf("outbox: cleanup: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("outbox: cleanup rows affected: %w", err)
	}

	return n, nil
}

// CountPending returns the number of entries awaiting delivery (unpublished,
// including both unclaimed and stale-claimed entries).
func (s *Store) CountPending(ctx context.Context) (int64, error) {
	const query = `SELECT COUNT(*) FROM outbox_events WHERE published_at IS NULL`
	var count int64
	if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, fmt.Errorf("outbox: count pending: %w", err)
	}
	return count, nil
}
