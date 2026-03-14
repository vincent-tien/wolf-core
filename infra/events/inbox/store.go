// store.go — Inbox store for at-least-once message processing with idempotent deduplication.
//
// The inbox pattern complements the outbox: outbox guarantees at-least-once
// publishing; inbox provides idempotent deduplication so that handlers process
// each message effectively once under normal operation. Handlers must be idempotent
// because the dedup marker is written AFTER successful handler execution.
package inbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// InboxStore provides idempotent message processing via the inbox_events table.
// Each message ID is recorded with INSERT ON CONFLICT DO NOTHING; a zero
// rows-affected result means the message has already been processed and should
// be skipped by the caller.
//
// InboxStore uses its own connection pool and does not participate in the
// caller's transaction. The dedup marker is written AFTER handler success.
// If the app crashes between handler completion and marker write, the broker
// redelivers and the handler runs again — handlers must be idempotent.
type InboxStore struct {
	db *sql.DB
}

// NewInboxStore creates an *InboxStore backed by the provided connection pool.
func NewInboxStore(db *sql.DB) *InboxStore {
	return &InboxStore{db: db}
}

// IsProcessed checks whether the given messageID has already been recorded.
func (s *InboxStore) IsProcessed(ctx context.Context, messageID string) (bool, error) {
	const query = `SELECT EXISTS(SELECT 1 FROM inbox_events WHERE id = $1)`
	var exists bool
	if err := s.db.QueryRowContext(ctx, query, messageID).Scan(&exists); err != nil {
		return false, fmt.Errorf("inbox store: is processed %q: %w", messageID, err)
	}
	return exists, nil
}

// MarkProcessed attempts to insert messageID into the inbox_events table.
// It returns (true, nil) when the row was inserted successfully, indicating
// this is the first time the message has been seen. It returns (false, nil)
// when ON CONFLICT fires, indicating the message is a duplicate. Any database
// error is returned as (false, err).
func (s *InboxStore) MarkProcessed(ctx context.Context, messageID, eventType string) (bool, error) {
	const query = `
		INSERT INTO inbox_events (id, event_type, processed_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (id) DO NOTHING`

	result, err := s.db.ExecContext(ctx, query, messageID, eventType)
	if err != nil {
		return false, fmt.Errorf("inbox store: mark processed %q: %w", messageID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inbox store: rows affected %q: %w", messageID, err)
	}

	return rows > 0, nil
}

// Cleanup deletes inbox records older than the given retention duration.
// Returns the number of rows deleted. Designed to be called periodically
// (e.g. daily) to prevent unbounded table growth.
func (s *InboxStore) Cleanup(ctx context.Context, retention time.Duration) (int64, error) {
	const query = `DELETE FROM inbox_events WHERE processed_at < $1`
	cutoff := time.Now().UTC().Add(-retention)
	result, err := s.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("inbox store: cleanup: %w", err)
	}
	return result.RowsAffected()
}
