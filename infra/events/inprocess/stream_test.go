package inprocess_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/events/inprocess"
	"github.com/vincent-tien/wolf-core/messaging"
)

// newStream is a test helper that returns a ready-to-use *InprocessStream
// using a no-op logger. It registers a cleanup hook to close the stream.
func newStream(t *testing.T) *inprocess.InprocessStream {
	t.Helper()
	s := inprocess.NewStream(zap.NewNop())
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// --- messaging.Stream contract ---

func TestInprocessStream_ImplementsStreamInterface(t *testing.T) {
	var _ messaging.Stream = inprocess.NewStream(zap.NewNop())
}

func TestInprocessStream_PublishSubscribe_DeliversSingleMessage(t *testing.T) {
	// Arrange
	s := newStream(t)
	subject := "inprocess.deliver"
	want := []byte("payload-bytes")
	delivered := make(chan []byte, 1)

	err := s.Subscribe(subject, func(_ context.Context, msg messaging.Message) error {
		delivered <- msg.Data()
		return nil
	})
	require.NoError(t, err)

	// Act
	err = s.Publish(context.Background(), subject, messaging.RawMessage{
		Subject: subject,
		Data:    want,
	})
	require.NoError(t, err)

	// Assert
	select {
	case got := <-delivered:
		assert.Equal(t, want, got)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivery")
	}
}

func TestInprocessStream_MultipleSubscribers_AllReceive(t *testing.T) {
	// Arrange
	s := newStream(t)
	subject := "inprocess.fanout"
	const n = 4

	var mu sync.Mutex
	var deliveries [][]byte

	for i := 0; i < n; i++ {
		err := s.Subscribe(subject, func(_ context.Context, msg messaging.Message) error {
			mu.Lock()
			deliveries = append(deliveries, msg.Data())
			mu.Unlock()
			return nil
		})
		require.NoError(t, err)
	}

	// Act
	payload := []byte("multi-sub-data")
	require.NoError(t, s.Publish(context.Background(), subject, messaging.RawMessage{
		Subject: subject,
		Data:    payload,
	}))

	// Assert
	assert.Len(t, deliveries, n)
	for _, d := range deliveries {
		assert.Equal(t, payload, d)
	}
}

func TestInprocessStream_SubjectIsolation(t *testing.T) {
	// Arrange
	s := newStream(t)
	target := "inprocess.target"
	other := "inprocess.other"
	called := false

	require.NoError(t, s.Subscribe(target, func(_ context.Context, msg messaging.Message) error {
		called = true
		return nil
	}))

	// Act — publish on different subject
	require.NoError(t, s.Publish(context.Background(), other, messaging.RawMessage{
		Subject: other,
		Data:    []byte("wrong-subject"),
	}))

	// Assert
	assert.False(t, called)
}

func TestInprocessStream_NoSubscribers_PublishReturnsNil(t *testing.T) {
	s := newStream(t)
	err := s.Publish(context.Background(), "inprocess.empty", messaging.RawMessage{
		Data: []byte("no one home"),
	})
	assert.NoError(t, err)
}

func TestInprocessStream_HandlerError_LoggedNotPropagated(t *testing.T) {
	// Arrange — handler returns an error; Publish must still return nil
	s := newStream(t)
	subject := "inprocess.handler-error"

	require.NoError(t, s.Subscribe(subject, func(_ context.Context, msg messaging.Message) error {
		return errors.New("handler failure")
	}))

	// Act + Assert — error is logged internally; Publish returns nil
	err := s.Publish(context.Background(), subject, messaging.RawMessage{
		Subject: subject,
		Data:    []byte("trigger error"),
	})
	assert.NoError(t, err)
}

func TestInprocessStream_MessageFields_PreservedOnDelivery(t *testing.T) {
	// Arrange
	s := newStream(t)
	subject := "inprocess.fields"
	wantID := "msg-id-42"
	wantData := []byte(`{"key":"value"}`)
	wantHeaders := map[string]string{"event_type": "thing.happened.v1"}

	received := make(chan messaging.Message, 1)
	require.NoError(t, s.Subscribe(subject, func(_ context.Context, msg messaging.Message) error {
		received <- msg
		return nil
	}))

	// Act
	require.NoError(t, s.Publish(context.Background(), subject, messaging.RawMessage{
		ID:      wantID,
		Subject: subject,
		Data:    wantData,
		Headers: wantHeaders,
	}))

	// Assert
	select {
	case msg := <-received:
		assert.Equal(t, wantID, msg.ID())
		assert.Equal(t, subject, msg.Subject())
		assert.Equal(t, wantData, msg.Data())
		assert.Equal(t, wantHeaders, msg.Headers())
		assert.Equal(t, 1, msg.DeliveryAttempt())
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestInprocessMessage_Lifecycle_AllNoops(t *testing.T) {
	// Arrange
	s := newStream(t)
	subject := "inprocess.noops"
	received := make(chan messaging.Message, 1)

	require.NoError(t, s.Subscribe(subject, func(_ context.Context, msg messaging.Message) error {
		received <- msg
		return nil
	}))
	require.NoError(t, s.Publish(context.Background(), subject, messaging.RawMessage{
		Subject: subject,
		Data:    []byte("x"),
	}))

	// Act + Assert — all lifecycle calls return nil (no-op)
	select {
	case msg := <-received:
		assert.NoError(t, msg.Ack())
		assert.NoError(t, msg.Nak())
		assert.NoError(t, msg.NakWithDelay(5*time.Second))
		assert.NoError(t, msg.Term())
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestInprocessStream_Close_ReturnsNil(t *testing.T) {
	s := inprocess.NewStream(zap.NewNop())
	assert.NoError(t, s.Close())
}

func TestInprocessStream_SubscribeOptions_Accepted(t *testing.T) {
	// Verify SubscribeOption variadic args are accepted without error even
	// though the in-process adapter ignores them.
	s := newStream(t)
	err := s.Subscribe("inprocess.opts",
		func(_ context.Context, msg messaging.Message) error { return nil },
		messaging.WithGroup("g1"),
		messaging.WithDurable("d1"),
		messaging.WithMaxDeliver(3),
		messaging.WithMaxAckPending(50),
		messaging.WithAckWait(15*time.Second),
	)
	assert.NoError(t, err)
}
