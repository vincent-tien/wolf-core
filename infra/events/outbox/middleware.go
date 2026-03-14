// middleware.go — Wraps messaging.Publisher to insert into outbox table instead of publishing directly.
package outbox

import (
	"context"
	"fmt"

	"github.com/vincent-tien/wolf-core/messaging"
)

// EntryInserter is the minimal interface OutboxMiddleware requires from the
// outbox store. *Store satisfies this interface. The interface is exported so
// test packages can supply fakes via NewOutboxMiddlewareWithInserter.
type EntryInserter interface {
	InsertEntry(ctx context.Context, entry OutboxEntry) error
}

// OutboxMiddleware implements messaging.Publisher by writing messages to the
// outbox table instead of dispatching them directly to a broker.
//
// This is the transactional outbox pattern entry point for the messaging layer.
// When callers publish a RawMessage through this middleware, the message is
// persisted durably in the same database as the domain state (via InsertEntry),
// and the outbox relay worker forwards it to the actual broker asynchronously.
//
// Header conventions: the message headers map may contain the following keys
// that are mapped to OutboxEntry fields:
//
//	"aggregate_type" → AggregateType
//	"aggregate_id"   → AggregateID
//	"event_type"     → EventType  (falls back to msg.Name when absent)
//	"trace_id"       → TraceID
type OutboxMiddleware struct {
	store EntryInserter
}

// NewOutboxMiddleware constructs an OutboxMiddleware backed by store.
func NewOutboxMiddleware(store *Store) *OutboxMiddleware {
	return &OutboxMiddleware{store: store}
}

// NewOutboxMiddlewareWithInserter constructs an OutboxMiddleware from any
// EntryInserter implementation. This constructor is exported for use in tests
// that supply a fake store without a live database connection.
func NewOutboxMiddlewareWithInserter(store EntryInserter) *OutboxMiddleware {
	return &OutboxMiddleware{store: store}
}

// Publish writes msg to the outbox table. The subject parameter is
// stored as the event_type when the "event_type" header is absent. The method
// satisfies messaging.Publisher and is safe for concurrent use.
func (m *OutboxMiddleware) Publish(ctx context.Context, subject string, msg messaging.RawMessage) error {
	id := msg.ID
	if id == "" {
		return fmt.Errorf("outbox middleware: message ID must not be empty")
	}

	eventType := msg.Headers["event_type"]
	if eventType == "" {
		// Fall back to the human-readable name, then to the subject.
		eventType = msg.Name
		if eventType == "" {
			eventType = subject
		}
	}

	entry := OutboxEntry{
		ID:            id,
		AggregateType: msg.Headers["aggregate_type"],
		AggregateID:   msg.Headers["aggregate_id"],
		EventType:     eventType,
		Payload:       msg.Data,
		TraceID:       msg.Headers["trace_id"],
	}

	if err := m.store.InsertEntry(ctx, entry); err != nil {
		return fmt.Errorf("outbox middleware: publish %q: %w", id, err)
	}

	return nil
}
