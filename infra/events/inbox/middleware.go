// middleware.go — Inbox deduplication middleware for messaging handlers.
//
// This is the primary integration point — consumers wrap their handlers
// with InboxMiddleware.Wrap() at subscription time.
package inbox

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/messaging"
)

// Deduplicator is the minimal interface InboxMiddleware requires from the
// inbox store. *InboxStore satisfies this interface. The interface is exported
// so test packages can supply fakes via NewInboxMiddlewareWithDeduplicator.
type Deduplicator interface {
	// IsProcessed returns true if the message has already been processed.
	IsProcessed(ctx context.Context, messageID string) (bool, error)
	// MarkProcessed records the message as processed. Returns (true, nil) if newly
	// inserted, (false, nil) if already existed (duplicate).
	MarkProcessed(ctx context.Context, messageID, eventType string) (bool, error)
}

// InboxMiddleware wraps a messaging.MessageHandler with idempotent processing.
// Before invoking the inner handler it calls deduplicator.MarkProcessed; if the
// message has already been seen (duplicate), the inner handler is skipped and
// nil is returned. This implements the transactional inbox pattern for the
// messaging layer, preventing duplicate side-effects caused by at-least-once
// broker delivery.
//
// The event_type header is read from the message to populate the inbox record.
// When absent, the message subject is used as a fallback.
type InboxMiddleware struct {
	store       Deduplicator
	logger      *zap.Logger
	dedupTotal  CounterIncrementer // optional: incremented on each deduplicated message
}

// CounterIncrementer is satisfied by prometheus.Counter and test stubs.
type CounterIncrementer interface {
	Inc()
}

// NewInboxMiddleware constructs an InboxMiddleware backed by store.
func NewInboxMiddleware(store *InboxStore, logger *zap.Logger) *InboxMiddleware {
	return &InboxMiddleware{store: store, logger: logger}
}

// NewInboxMiddlewareWithDeduplicator constructs an InboxMiddleware from any
// Deduplicator implementation. This constructor is exported for use in tests
// that supply a fake store without a live database connection.
func NewInboxMiddlewareWithDeduplicator(store Deduplicator, logger *zap.Logger) *InboxMiddleware {
	return &InboxMiddleware{store: store, logger: logger}
}

// WithDedupCounter attaches a Prometheus counter that increments on each
// deduplicated (skipped) message. Recommended metric name: wolf_inbox_deduplicated_total.
func (m *InboxMiddleware) WithDedupCounter(c CounterIncrementer) *InboxMiddleware {
	m.dedupTotal = c
	return m
}

// Wrap returns a new MessageHandler that provides at-least-once processing with
// idempotent deduplication. The handler runs FIRST; only on success is the inbox
// marker written. On crash between handler success and marker write, the broker
// redelivers and the handler runs again (handlers must be idempotent). Once the
// marker is written, subsequent deliveries are deduplicated via INSERT ON CONFLICT.
func (m *InboxMiddleware) Wrap(next messaging.MessageHandler) messaging.MessageHandler {
	return func(ctx context.Context, msg messaging.Message) error {
		eventType := msg.Headers()["event_type"]
		if eventType == "" {
			eventType = msg.Subject()
		}

		// Pre-flight dedup: skip if already processed (optimistic fast-path).
		// NOTE: Under concurrent redelivery of the same message ID, two goroutines
		// can both pass this check, both run the handler, and one wins the
		// MarkProcessed INSERT. This is by design — handlers MUST be idempotent.
		// The pre-flight check optimizes sequential duplicates, not concurrent ones.
		alreadyProcessed, err := m.store.IsProcessed(ctx, msg.ID())
		if err != nil {
			return fmt.Errorf("inbox middleware: dedup check for %q: %w", msg.ID(), err)
		}
		if alreadyProcessed {
			m.logger.Debug("inbox middleware: duplicate message skipped",
				zap.String("message_id", msg.ID()),
				zap.String("event_type", eventType),
			)
			if m.dedupTotal != nil {
				m.dedupTotal.Inc()
			}
			return nil
		}

		// Run handler first — if it fails, we don't mark processed so broker retries.
		if err := next(ctx, msg); err != nil {
			return err
		}

		// Mark processed after successful handler execution.
		if _, markErr := m.store.MarkProcessed(ctx, msg.ID(), eventType); markErr != nil {
			m.logger.Warn("inbox middleware: failed to mark processed (handler succeeded, will dedup on retry)",
				zap.String("message_id", msg.ID()),
				zap.Error(markErr),
			)
		}

		return nil
	}
}
