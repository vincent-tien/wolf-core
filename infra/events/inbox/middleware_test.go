package inbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/events/inbox"
	"github.com/vincent-tien/wolf-core/messaging"
)

// --- fakes ---

// fakeDeduplicator simulates the InboxStore for unit tests.
// It implements inbox.Deduplicator.
type fakeDeduplicator struct {
	seen map[string]struct{}
	err  error
}

var _ inbox.Deduplicator = (*fakeDeduplicator)(nil)

func newFakeDeduplicator() *fakeDeduplicator {
	return &fakeDeduplicator{seen: make(map[string]struct{})}
}

func (f *fakeDeduplicator) IsProcessed(_ context.Context, messageID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, exists := f.seen[messageID]
	return exists, nil
}

func (f *fakeDeduplicator) MarkProcessed(_ context.Context, messageID, _ string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if _, exists := f.seen[messageID]; exists {
		return false, nil
	}
	f.seen[messageID] = struct{}{}
	return true, nil
}

// inprocessMessage is a minimal messaging.Message test double.
type inprocessMessage struct {
	id      string
	subject string
	data    []byte
	headers map[string]string
}

var _ messaging.Message = (*inprocessMessage)(nil)

func (m *inprocessMessage) ID() string                       { return m.id }
func (m *inprocessMessage) Subject() string                  { return m.subject }
func (m *inprocessMessage) Data() []byte                     { return m.data }
func (m *inprocessMessage) Headers() map[string]string       { return m.headers }
func (m *inprocessMessage) Ack() error                       { return nil }
func (m *inprocessMessage) Nak() error                       { return nil }
func (m *inprocessMessage) NakWithDelay(_ time.Duration) error { return nil }
func (m *inprocessMessage) Term() error                      { return nil }
func (m *inprocessMessage) DeliveryAttempt() int             { return 1 }

// --- helpers ---

func newMW(t *testing.T, store inbox.Deduplicator) *inbox.InboxMiddleware {
	t.Helper()
	return inbox.NewInboxMiddlewareWithDeduplicator(store, zap.NewNop())
}

// --- tests ---

func TestInboxMiddleware_FirstDelivery_InvokesHandler(t *testing.T) {
	// Arrange
	dedup := newFakeDeduplicator()
	mw := newMW(t, dedup)

	msg := &inprocessMessage{
		id:      "msg-001",
		subject: "orders",
		headers: map[string]string{"event_type": "order.created.v1"},
		data:    []byte(`{"order_id":"o-1"}`),
	}

	called := false
	handler := func(_ context.Context, _ messaging.Message) error {
		called = true
		return nil
	}

	// Act
	err := mw.Wrap(handler)(context.Background(), msg)

	// Assert
	require.NoError(t, err)
	assert.True(t, called, "handler should be called on first delivery")
}

func TestInboxMiddleware_DuplicateDelivery_SkipsHandler(t *testing.T) {
	// Arrange
	dedup := newFakeDeduplicator()
	mw := newMW(t, dedup)

	msg := &inprocessMessage{
		id:      "msg-dup",
		subject: "orders",
		headers: map[string]string{"event_type": "order.created.v1"},
		data:    []byte(`{}`),
	}

	callCount := 0
	handler := func(_ context.Context, _ messaging.Message) error {
		callCount++
		return nil
	}

	wrapped := mw.Wrap(handler)

	// Act — deliver same message twice
	require.NoError(t, wrapped(context.Background(), msg))
	require.NoError(t, wrapped(context.Background(), msg))

	// Assert — handler must only be called once
	assert.Equal(t, 1, callCount, "handler should be called only on first delivery")
}

func TestInboxMiddleware_EventTypeFallsBackToSubject(t *testing.T) {
	// Arrange — no "event_type" header; subject should be used as event type.
	dedup := newFakeDeduplicator()
	mw := newMW(t, dedup)

	msg := &inprocessMessage{
		id:      "msg-003",
		subject: "inventory",
		headers: map[string]string{}, // no event_type header
		data:    []byte(`{}`),
	}

	called := false
	handler := func(_ context.Context, _ messaging.Message) error {
		called = true
		return nil
	}

	// Act
	err := mw.Wrap(handler)(context.Background(), msg)

	// Assert
	require.NoError(t, err)
	assert.True(t, called)
}

func TestInboxMiddleware_HandlerError_NotMarkedProcessed(t *testing.T) {
	// Arrange — handler fails, message should NOT be marked processed so broker retries.
	dedup := newFakeDeduplicator()
	mw := newMW(t, dedup)

	msg := &inprocessMessage{
		id:      "msg-004",
		subject: "orders",
		headers: map[string]string{"event_type": "order.created.v1"},
		data:    []byte(`{}`),
	}

	handlerErr := errors.New("downstream service unavailable")
	handler := func(_ context.Context, _ messaging.Message) error {
		return handlerErr
	}

	// Act
	err := mw.Wrap(handler)(context.Background(), msg)

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, handlerErr)
	// Message should NOT be in seen set — retry must work
	_, inSeen := dedup.seen[msg.id]
	assert.False(t, inSeen, "failed message must not be marked processed")
}

func TestInboxMiddleware_StoreError_Propagated(t *testing.T) {
	// Arrange — the deduplication store returns an error; it must be propagated
	// and the inner handler must NOT be called.
	storeErr := errors.New("db: connection refused")
	dedup := &fakeDeduplicator{seen: make(map[string]struct{}), err: storeErr}
	mw := newMW(t, dedup)

	msg := &inprocessMessage{
		id:      "msg-005",
		subject: "orders",
		headers: map[string]string{"event_type": "order.created.v1"},
		data:    []byte(`{}`),
	}

	called := false
	handler := func(_ context.Context, _ messaging.Message) error {
		called = true
		return nil
	}

	// Act
	err := mw.Wrap(handler)(context.Background(), msg)

	// Assert
	require.Error(t, err)
	assert.ErrorIs(t, err, storeErr)
	assert.False(t, called, "handler must not be called when store errors")
}

func TestInboxMiddleware_DifferentIDs_BothProcessed(t *testing.T) {
	// Arrange — two distinct messages should both be processed.
	dedup := newFakeDeduplicator()
	mw := newMW(t, dedup)

	callCount := 0
	handler := func(_ context.Context, _ messaging.Message) error {
		callCount++
		return nil
	}
	wrapped := mw.Wrap(handler)

	msg1 := &inprocessMessage{id: "msg-a", subject: "s", headers: map[string]string{}}
	msg2 := &inprocessMessage{id: "msg-b", subject: "s", headers: map[string]string{}}

	// Act
	require.NoError(t, wrapped(context.Background(), msg1))
	require.NoError(t, wrapped(context.Background(), msg2))

	// Assert
	assert.Equal(t, 2, callCount, "both distinct messages should be processed")
}
