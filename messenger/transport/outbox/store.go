// Package outbox implements a messenger transport backed by the PostgreSQL
// outbox_events table. Send inserts within the caller's transaction (atomic
// with domain writes). Get polls unpublished rows for relay processing.
package outbox

import (
	"context"
	"time"
)

// Entry is a single row in outbox_events as seen by the messenger transport.
type Entry struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	TraceID       string
	CreatedAt     time.Time
	RetryCount    int
}

// Writer inserts outbox entries. Implementations SHOULD extract the
// transaction from ctx (via tx.Extract) so the insert is atomic with the
// caller's domain write.
type Writer interface {
	Insert(ctx context.Context, entry Entry) error
}

// Reader reads and manages outbox entries for relay processing.
type Reader interface {
	GetUnpublished(ctx context.Context, batchSize int) ([]Entry, error)
	MarkPublished(ctx context.Context, id string) error
	IncrementRetry(ctx context.Context, id string, lastError string) error
}

// Store combines Writer and Reader for full outbox lifecycle.
type Store interface {
	Writer
	Reader
}
