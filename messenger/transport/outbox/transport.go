// transport.go — Outbox transport implementation (Send inserts, Get polls).
package outbox

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/serde"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
	"github.com/vincent-tien/wolf-core/messenger/transport"
)

var _ transport.Transport = (*Transport)(nil)

// Transport implements messenger transport.Sender and transport.Receiver
// backed by the outbox_events PostgreSQL table.
//
// Send path: serialize envelope → insert into outbox_events (tx-aware).
// Get path: poll unpublished entries → deserialize → attach OutboxIDStamp.
// Ack: mark entry published.
// Reject: increment retry count.
type Transport struct {
	writer     Writer
	reader     Reader
	serializer serde.Serializer
	batchSize  int
	logger     *slog.Logger
}

// New creates an outbox transport. Accepts Writer and Reader separately so
// callers can use a single PostgresStore for both, or split for testing.
func New(store Store, serializer serde.Serializer, opts ...Option) *Transport {
	t := &Transport{
		writer:     store,
		reader:     store,
		serializer: serializer,
		batchSize:  100,
		logger:     slog.Default(),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *Transport) Name() string { return "outbox" }

// Send serializes the envelope and inserts it into outbox_events.
// If a transaction exists in ctx (via tx.Inject), the insert is atomic
// with the caller's domain write.
func (t *Transport) Send(ctx context.Context, env messenger.Envelope) error {
	data, err := t.serializer.Encode(env)
	if err != nil {
		return fmt.Errorf("outbox transport: encode: %w", err)
	}

	var aggType, aggID string
	if as := env.Last(stamp.NameAggregate); as != nil {
		agg := as.(stamp.AggregateStamp)
		aggType = agg.Type
		aggID = agg.ID
	}

	var traceID string
	if ts := env.Last(stamp.NameTrace); ts != nil {
		traceID = ts.(stamp.TraceStamp).TraceID
	}

	entry := Entry{
		ID:            uuid.NewString(),
		AggregateType: aggType,
		AggregateID:   aggID,
		EventType:     messenger.TypeNameOf(env.Message),
		Payload:       data,
		TraceID:       traceID,
		CreatedAt:     env.CreatedAt(),
	}

	if err := t.writer.Insert(ctx, entry); err != nil {
		return fmt.Errorf("outbox transport: insert: %w", err)
	}
	return nil
}

// Get polls unpublished outbox entries and deserializes them back to envelopes.
// Each envelope receives an OutboxIDStamp for Ack/Reject correlation.
func (t *Transport) Get(ctx context.Context) ([]messenger.Envelope, error) {
	entries, err := t.reader.GetUnpublished(ctx, t.batchSize)
	if err != nil {
		return nil, fmt.Errorf("outbox transport: get: %w", err)
	}

	if len(entries) == 0 {
		return nil, nil
	}

	envelopes := make([]messenger.Envelope, 0, len(entries))
	for _, e := range entries {
		env, err := t.serializer.Decode(e.Payload)
		if err != nil {
			t.logger.Error("outbox transport: decode failed, incrementing retry",
				slog.String("entry_id", e.ID),
				slog.String("event_type", e.EventType),
				slog.String("error", err.Error()),
			)
			if retryErr := t.reader.IncrementRetry(ctx, e.ID, "decode: "+err.Error()); retryErr != nil {
				t.logger.Error("outbox transport: increment retry on decode failure",
					slog.String("entry_id", e.ID),
					slog.String("error", retryErr.Error()),
				)
			}
			continue
		}
		env = env.WithStamp(stamp.OutboxIDStamp{EntryID: e.ID})
		envelopes = append(envelopes, env)
	}
	return envelopes, nil
}

// Ack marks the outbox entry as published.
func (t *Transport) Ack(ctx context.Context, env messenger.Envelope) error {
	id, err := outboxEntryID(env)
	if err != nil {
		return fmt.Errorf("outbox transport: ack: %w", err)
	}
	if err := t.reader.MarkPublished(ctx, id); err != nil {
		return fmt.Errorf("outbox transport: ack: %w", err)
	}
	return nil
}

// Reject increments the retry count and records the failure reason.
func (t *Transport) Reject(ctx context.Context, env messenger.Envelope, reason error) error {
	id, err := outboxEntryID(env)
	if err != nil {
		return fmt.Errorf("outbox transport: reject: %w", err)
	}
	errMsg := ""
	if reason != nil {
		errMsg = reason.Error()
	}
	if err := t.reader.IncrementRetry(ctx, id, errMsg); err != nil {
		return fmt.Errorf("outbox transport: reject: %w", err)
	}
	return nil
}

// Close is a no-op — the outbox_events table is managed externally.
func (t *Transport) Close() error { return nil }

func outboxEntryID(env messenger.Envelope) (string, error) {
	s := env.Last(stamp.NameOutboxID)
	if s == nil {
		return "", fmt.Errorf("outbox transport: missing OutboxIDStamp on envelope")
	}
	return s.(stamp.OutboxIDStamp).EntryID, nil
}
