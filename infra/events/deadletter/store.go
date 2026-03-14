// Package deadletter provides a dead letter queue (DLQ) for messages that
// have exhausted all delivery retry attempts. Entries are persisted in the
// dead_letter_queue table and can be inspected, re-queued, or discarded by
// operators.
package deadletter

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	sharedErrors "github.com/vincent-tien/wolf-core/errors"
)

// DLQEntry represents a single row in the dead_letter_queue table. It captures
// the full message content plus failure metadata so the entry can be diagnosed
// and replayed without any information loss.
type DLQEntry struct {
	// ID is the original message identifier.
	ID string
	// Subject is the topic/subject the message was published on.
	Subject string
	// Data is the raw serialised payload bytes.
	Data []byte
	// Headers is the JSON-encoded map of message headers.
	Headers []byte
	// Error is the last error message that caused delivery failure.
	Error string
	// Attempts is the total number of delivery attempts made.
	Attempts int
	// OriginalAt is the timestamp when the message was first created.
	OriginalAt time.Time
	// DeadAt is the timestamp when the message was moved to the DLQ.
	DeadAt time.Time
}

// Store provides dead letter queue operations backed by the dead_letter_queue
// table. All operations use the provided connection pool directly; DLQ writes
// intentionally occur outside the domain transaction because the failure has
// already been committed (or lost) by the time the entry moves to the DLQ.
type Store struct {
	db *sql.DB
}

// NewStore creates a *Store backed by the provided connection pool.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Insert adds a failed entry to the dead letter queue. It is called by the
// outbox relay worker when an entry's retry count reaches the configured
// maximum. The entry must have a non-empty ID; duplicate IDs are rejected by
// the primary key constraint.
func (s *Store) Insert(ctx context.Context, entry DLQEntry) error {
	const query = `
		INSERT INTO dead_letter_queue
			(id, subject, data, headers, error, attempts, original_at, dead_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (id) DO NOTHING`

	_, err := s.db.ExecContext(ctx, query,
		entry.ID,
		entry.Subject,
		entry.Data,
		entry.Headers,
		entry.Error,
		entry.Attempts,
		entry.OriginalAt,
	)
	if err != nil {
		return fmt.Errorf("deadletter: insert %q: %w", entry.ID, err)
	}

	return nil
}

// GetDeadLetters retrieves up to limit entries from the dead letter queue,
// ordered by dead_at ascending so the oldest failures are surfaced first.
func (s *Store) GetDeadLetters(ctx context.Context, limit int) (_ []DLQEntry, err error) {
	const query = `
		SELECT id, subject, data, headers, error, attempts, original_at, dead_at
		FROM   dead_letter_queue
		ORDER  BY dead_at ASC
		LIMIT  $1`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("deadletter: query: %w", err)
	}
	defer sharedErrors.Do(&err, rows.Close, "close rows")

	entries := make([]DLQEntry, 0, limit)
	for rows.Next() {
		var e DLQEntry
		if err := rows.Scan(
			&e.ID, &e.Subject, &e.Data, &e.Headers,
			&e.Error, &e.Attempts, &e.OriginalAt, &e.DeadAt,
		); err != nil {
			return nil, fmt.Errorf("deadletter: scan row: %w", err)
		}
		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deadletter: iterate rows: %w", err)
	}

	return entries, nil
}

// Retry removes the DLQ entry identified by id. The caller is responsible for
// re-publishing the message before or after calling Retry. The deletion is
// idempotent: no error is returned if the id does not exist.
func (s *Store) Retry(ctx context.Context, id string) error {
	const query = `DELETE FROM dead_letter_queue WHERE id = $1`

	if _, err := s.db.ExecContext(ctx, query, id); err != nil {
		return fmt.Errorf("deadletter: retry (delete) %q: %w", id, err)
	}

	return nil
}

// MarshalHeaders serialises a string map to JSON bytes suitable for the
// headers column. Callers may use this helper when constructing a DLQEntry
// from a messaging.RawMessage.
func MarshalHeaders(headers map[string]string) ([]byte, error) {
	if len(headers) == 0 {
		return []byte("{}"), nil
	}

	b, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("deadletter: marshal headers: %w", err)
	}

	return b, nil
}
