// Package inbox provides idempotent event consumption via two complementary patterns:
//
// Processor — insert-first (at-most-once) for the in-process domain event bus.
// Records the event ID BEFORE invoking the handler. If the handler fails after
// insertion, the event is considered processed and will NOT be retried. This is
// acceptable for in-process events which are already lost on process crash.
//
// InboxMiddleware — handler-first (at-least-once) for broker-delivered messages.
// Runs the handler FIRST; only on success is the inbox marker written. Use this
// for external messages where redelivery on failure is required.
package inbox

import (
	"context"
	"database/sql"
	"fmt"

	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/event"
)

// Processor provides idempotent event handling backed by a database dedup
// table. Each event is recorded by its ID before the handler is called; a
// subsequent delivery of the same event ID is silently ignored.
type Processor struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewProcessor creates a *Processor backed by the provided connection pool.
func NewProcessor(db *sql.DB, logger *zap.Logger) *Processor {
	return &Processor{db: db, logger: logger}
}

// Process invokes handler for evt if and only if evt.EventID() has not been
// processed before. Deduplication is implemented via an INSERT ON CONFLICT DO
// NOTHING against the inbox_events table; if zero rows are inserted the event
// has already been processed and the handler is skipped.
//
// handler is called within the same goroutine as Process; any error it returns
// is propagated directly to the caller.
func (p *Processor) Process(ctx context.Context, evt event.Event, handler event.EventHandler) error {
	const query = `
		INSERT INTO inbox_events (id, event_type, processed_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (id) DO NOTHING`

	result, err := p.db.ExecContext(ctx, query, evt.EventID(), evt.EventType())
	if err != nil {
		return fmt.Errorf("inbox: record event %q: %w", evt.EventID(), err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inbox: rows affected %q: %w", evt.EventID(), err)
	}

	// rows == 0 means ON CONFLICT fired — event was already processed.
	if rows == 0 {
		p.logger.Debug("inbox: duplicate event skipped",
			zap.String("event_id", evt.EventID()),
			zap.String("event_type", evt.EventType()),
		)
		return nil
	}

	if err := handler(ctx, evt); err != nil {
		return fmt.Errorf("inbox: handler error for event %q: %w", evt.EventID(), err)
	}

	return nil
}
