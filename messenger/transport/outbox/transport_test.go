package outbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/serde"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
	"github.com/vincent-tien/wolf-core/messenger/transport/outbox"
)

// ── test types ──

type placeOrder struct {
	OrderID string `json:"order_id"`
}

func (placeOrder) MessageName() string { return "order.PlaceOrder" }

var testLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

// ── fake store ──

type fakeStore struct {
	mu      sync.Mutex
	entries []outbox.Entry
}

func (f *fakeStore) Insert(_ context.Context, entry outbox.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry)
	return nil
}

func (f *fakeStore) GetUnpublished(_ context.Context, batchSize int) ([]outbox.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := batchSize
	if n > len(f.entries) {
		n = len(f.entries)
	}
	out := make([]outbox.Entry, n)
	copy(out, f.entries[:n])
	return out, nil
}

func (f *fakeStore) MarkPublished(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, e := range f.entries {
		if e.ID == id {
			f.entries = append(f.entries[:i], f.entries[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeStore) IncrementRetry(_ context.Context, id string, lastError string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, e := range f.entries {
		if e.ID == id {
			f.entries[i].RetryCount++
			_ = lastError
			return nil
		}
	}
	return nil
}

func newTestSerializer() serde.Serializer {
	reg := serde.NewTypeRegistry()
	serde.RegisterMessage[placeOrder](reg, "order.PlaceOrder", 1)
	return serde.NewJSONSerializer(reg, "test")
}

func newTestTransport(store outbox.Store) *outbox.Transport {
	return outbox.New(store, newTestSerializer(),
		outbox.WithLogger(testLogger),
		outbox.WithBatchSize(10),
	)
}

// ── tests ──

func TestTransport_SendAndGet(t *testing.T) {
	store := &fakeStore{}
	tr := newTestTransport(store)

	env := messenger.NewEnvelope(placeOrder{OrderID: "ord-1"},
		stamp.AggregateStamp{Type: "Order", ID: "ord-1"},
		stamp.TraceStamp{TraceID: "trace-abc"},
	)

	if err := tr.Send(context.Background(), env); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if len(store.entries) != 1 {
		t.Fatalf("store.entries = %d, want 1", len(store.entries))
	}

	entry := store.entries[0]
	if entry.AggregateType != "Order" {
		t.Errorf("AggregateType = %q, want %q", entry.AggregateType, "Order")
	}
	if entry.AggregateID != "ord-1" {
		t.Errorf("AggregateID = %q, want %q", entry.AggregateID, "ord-1")
	}
	if entry.EventType != "order.PlaceOrder" {
		t.Errorf("EventType = %q, want %q", entry.EventType, "order.PlaceOrder")
	}
	if entry.TraceID != "trace-abc" {
		t.Errorf("TraceID = %q, want %q", entry.TraceID, "trace-abc")
	}

	// Payload should be valid JSON wire envelope.
	var wire serde.WireEnvelope
	if err := json.Unmarshal(entry.Payload, &wire); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if wire.MessageType != "order.PlaceOrder" {
		t.Errorf("wire.MessageType = %q, want %q", wire.MessageType, "order.PlaceOrder")
	}

	// Get should return the envelope with OutboxIDStamp.
	envelopes, err := tr.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(envelopes) != 1 {
		t.Fatalf("envelopes = %d, want 1", len(envelopes))
	}

	got := envelopes[0]
	if !got.HasStamp(stamp.NameOutboxID) {
		t.Fatal("missing OutboxIDStamp")
	}

	cmd, ok := got.Message.(placeOrder)
	if !ok {
		t.Fatalf("message type = %T, want placeOrder", got.Message)
	}
	if cmd.OrderID != "ord-1" {
		t.Errorf("OrderID = %q, want %q", cmd.OrderID, "ord-1")
	}
}

func TestTransport_AckRemovesEntry(t *testing.T) {
	store := &fakeStore{}
	tr := newTestTransport(store)

	env := messenger.NewEnvelope(placeOrder{OrderID: "ack-test"})
	if err := tr.Send(context.Background(), env); err != nil {
		t.Fatalf("Send: %v", err)
	}

	envelopes, _ := tr.Get(context.Background())
	if len(envelopes) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(envelopes))
	}

	if err := tr.Ack(context.Background(), envelopes[0]); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	// After Ack, store should be empty.
	remaining, _ := tr.Get(context.Background())
	if len(remaining) != 0 {
		t.Errorf("expected 0 entries after Ack, got %d", len(remaining))
	}
}

func TestTransport_RejectIncrementsRetry(t *testing.T) {
	store := &fakeStore{}
	tr := newTestTransport(store)

	env := messenger.NewEnvelope(placeOrder{OrderID: "reject-test"})
	if err := tr.Send(context.Background(), env); err != nil {
		t.Fatalf("Send: %v", err)
	}

	envelopes, _ := tr.Get(context.Background())
	if err := tr.Reject(context.Background(), envelopes[0], errors.New("handler failed")); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	store.mu.Lock()
	if store.entries[0].RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", store.entries[0].RetryCount)
	}
	store.mu.Unlock()
}

func TestTransport_AckMissingStampReturnsError(t *testing.T) {
	store := &fakeStore{}
	tr := newTestTransport(store)

	env := messenger.NewEnvelope(placeOrder{OrderID: "no-stamp"})
	if err := tr.Ack(context.Background(), env); err == nil {
		t.Error("expected error for missing OutboxIDStamp")
	}
}

func TestTransport_SendNoAggregateStamp(t *testing.T) {
	store := &fakeStore{}
	tr := newTestTransport(store)

	env := messenger.NewEnvelope(placeOrder{OrderID: "no-agg"})
	if err := tr.Send(context.Background(), env); err != nil {
		t.Fatalf("Send: %v", err)
	}

	entry := store.entries[0]
	if entry.AggregateType != "" {
		t.Errorf("AggregateType should be empty, got %q", entry.AggregateType)
	}
	if entry.AggregateID != "" {
		t.Errorf("AggregateID should be empty, got %q", entry.AggregateID)
	}
}

func TestTransport_GetDecodeBadPayload(t *testing.T) {
	store := &fakeStore{
		entries: []outbox.Entry{
			{ID: "bad-1", Payload: []byte("not json")},
		},
	}
	tr := newTestTransport(store)

	envelopes, err := tr.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// Bad entry should be skipped, not fail the batch.
	if len(envelopes) != 0 {
		t.Errorf("expected 0 envelopes (bad entry skipped), got %d", len(envelopes))
	}
}

func TestTransport_Close(t *testing.T) {
	store := &fakeStore{}
	tr := newTestTransport(store)
	if err := tr.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestTransport_Name(t *testing.T) {
	store := &fakeStore{}
	tr := newTestTransport(store)
	if got := tr.Name(); got != "outbox" {
		t.Errorf("Name() = %q, want %q", got, "outbox")
	}
}

func TestTransport_TimestampPreserved(t *testing.T) {
	store := &fakeStore{}
	tr := newTestTransport(store)

	now := time.Now().Truncate(time.Millisecond)
	env := messenger.NewEnvelopeWithTime(placeOrder{OrderID: "ts-test"}, now)
	if err := tr.Send(context.Background(), env); err != nil {
		t.Fatalf("Send: %v", err)
	}

	envelopes, _ := tr.Get(context.Background())
	got := envelopes[0].CreatedAt().Truncate(time.Millisecond)
	if !got.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got, now)
	}
}
