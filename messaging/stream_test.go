package messaging_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/events/inprocess"
	"github.com/vincent-tien/wolf-core/messaging"
)

// newTestStream returns a messaging.Stream backed by the in-process adapter.
func newTestStream(t *testing.T) messaging.Stream {
	t.Helper()
	return inprocess.NewStream(zap.NewNop())
}

func TestStream_PublishSubscribe_DeliversSingleMessage(t *testing.T) {
	// Arrange
	stream := newTestStream(t)
	defer stream.Close()

	subject := "test.deliver"
	want := []byte("hello world")
	received := make(chan messaging.Message, 1)

	err := stream.Subscribe(subject, func(_ context.Context, msg messaging.Message) error {
		received <- msg
		return nil
	})
	require.NoError(t, err)

	// Act
	err = stream.Publish(context.Background(), subject, messaging.RawMessage{
		ID:      "msg-1",
		Subject: subject,
		Data:    want,
	})
	require.NoError(t, err)

	// Assert
	select {
	case msg := <-received:
		assert.Equal(t, want, msg.Data())
		assert.Equal(t, subject, msg.Subject())
		assert.Equal(t, "msg-1", msg.ID())
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message delivery")
	}
}

func TestStream_MultipleSubscribers_AllReceiveMessage(t *testing.T) {
	// Arrange
	stream := newTestStream(t)
	defer stream.Close()

	subject := "test.fanout"
	const numSubscribers = 3
	var mu sync.Mutex
	received := make([][]byte, 0, numSubscribers)

	for i := 0; i < numSubscribers; i++ {
		err := stream.Subscribe(subject, func(_ context.Context, msg messaging.Message) error {
			mu.Lock()
			received = append(received, msg.Data())
			mu.Unlock()
			return nil
		})
		require.NoError(t, err)
	}

	// Act
	payload := []byte("fanout-payload")
	err := stream.Publish(context.Background(), subject, messaging.RawMessage{
		Subject: subject,
		Data:    payload,
	})
	require.NoError(t, err)

	// Assert — all three subscribers received the payload
	assert.Len(t, received, numSubscribers)
	for _, data := range received {
		assert.Equal(t, payload, data)
	}
}

func TestStream_SubjectIsolation_HandlerNotCalledForOtherSubjects(t *testing.T) {
	// Arrange
	stream := newTestStream(t)
	defer stream.Close()

	targetSubject := "test.target"
	otherSubject := "test.other"
	called := false

	err := stream.Subscribe(targetSubject, func(_ context.Context, msg messaging.Message) error {
		called = true
		return nil
	})
	require.NoError(t, err)

	// Act — publish on a DIFFERENT subject
	err = stream.Publish(context.Background(), otherSubject, messaging.RawMessage{
		Subject: otherSubject,
		Data:    []byte("should-not-arrive"),
	})
	require.NoError(t, err)

	// Assert — handler registered on targetSubject must NOT have been called
	assert.False(t, called, "handler on %q must not be triggered by publish on %q", targetSubject, otherSubject)
}

func TestStream_PublishWithNoSubscribers_ReturnsNil(t *testing.T) {
	// Arrange
	stream := newTestStream(t)
	defer stream.Close()

	// Act + Assert — should succeed silently even with zero subscribers
	err := stream.Publish(context.Background(), "test.empty", messaging.RawMessage{
		Data: []byte("lonely message"),
	})
	assert.NoError(t, err)
}

func TestStream_Headers_DeliveredToHandler(t *testing.T) {
	// Arrange
	stream := newTestStream(t)
	defer stream.Close()

	subject := "test.headers"
	wantHeaders := map[string]string{
		"event_type": "order.placed.v1",
		"trace_id":   "abc-123",
	}
	received := make(chan map[string]string, 1)

	err := stream.Subscribe(subject, func(_ context.Context, msg messaging.Message) error {
		received <- msg.Headers()
		return nil
	})
	require.NoError(t, err)

	// Act
	err = stream.Publish(context.Background(), subject, messaging.RawMessage{
		Subject: subject,
		Data:    []byte("{}"),
		Headers: wantHeaders,
	})
	require.NoError(t, err)

	// Assert
	select {
	case got := <-received:
		assert.Equal(t, wantHeaders, got)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for headers")
	}
}

func TestMessage_AckNak_NoopForInprocess(t *testing.T) {
	// Arrange
	stream := newTestStream(t)
	defer stream.Close()

	subject := "test.ack"
	received := make(chan messaging.Message, 1)

	_ = stream.Subscribe(subject, func(_ context.Context, msg messaging.Message) error {
		received <- msg
		return nil
	})

	_ = stream.Publish(context.Background(), subject, messaging.RawMessage{
		Subject: subject,
		Data:    []byte("ack-test"),
	})

	// Act + Assert — all lifecycle methods are no-ops and return nil
	select {
	case msg := <-received:
		assert.NoError(t, msg.Ack())
		assert.NoError(t, msg.Nak())
		assert.NoError(t, msg.NakWithDelay(time.Second))
		assert.NoError(t, msg.Term())
		assert.Equal(t, 1, msg.DeliveryAttempt())
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestStream_SubscribeOptions_AcceptedWithoutError(t *testing.T) {
	// Arrange — verify all subscribe options can be passed without error
	stream := newTestStream(t)
	defer stream.Close()

	err := stream.Subscribe("test.opts",
		func(_ context.Context, msg messaging.Message) error { return nil },
		messaging.WithGroup("my-group"),
		messaging.WithDurable("my-durable"),
		messaging.WithMaxDeliver(5),
		messaging.WithMaxAckPending(100),
		messaging.WithAckWait(30*time.Second),
	)
	assert.NoError(t, err)
}
