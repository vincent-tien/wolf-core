// stream.go — In-memory messaging.Stream for development and testing.
package inprocess

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/messaging"
)

// InprocessStream is a goroutine-safe, in-memory implementation of
// messaging.Stream. It fans out published messages to all handlers registered
// for the same subject, invoking each handler synchronously in the calling
// goroutine.
//
// InprocessStream is intended for unit tests and local development. It provides
// no persistence or durability guarantees; messages are lost if no subscriber
// is registered at publish time. Ack/Nak/Term operations on received messages
// are no-ops because there is no broker to signal.
type InprocessStream struct {
	mu       sync.RWMutex
	handlers map[string][]messaging.MessageHandler
	logger   *zap.Logger
}

// NewStream creates and returns a ready-to-use *InprocessStream.
func NewStream(logger *zap.Logger) *InprocessStream {
	return &InprocessStream{
		handlers: make(map[string][]messaging.MessageHandler),
		logger:   logger,
	}
}

// Publish converts msg into an inprocessMessage and synchronously delivers it
// to all handlers registered for subject. Handler errors are logged but do not
// abort delivery to subsequent handlers.
func (s *InprocessStream) Publish(ctx context.Context, subject string, msg messaging.RawMessage) error {
	s.mu.RLock()
	src := s.handlers[subject]
	if len(src) == 0 {
		s.mu.RUnlock()
		return nil
	}
	handlers := make([]messaging.MessageHandler, len(src))
	copy(handlers, src)
	s.mu.RUnlock()

	m := &inprocessMessage{
		id:      msg.ID,
		subject: subject,
		data:    msg.Data,
		headers: msg.Headers,
	}

	for i, h := range handlers {
		if err := h(ctx, m); err != nil {
			s.logger.Error("inprocess stream: handler error",
				zap.String("subject", subject),
				zap.Int("handler_index", i),
				zap.Error(err),
			)
		}
	}
	return nil
}

// Subscribe registers handler for all messages published on subject.
// Multiple handlers may be registered for the same subject; all are called on
// each Publish. opts are accepted for interface compatibility but are ignored
// because in-process delivery provides no consumer group or durability
// semantics.
func (s *InprocessStream) Subscribe(subject string, handler messaging.MessageHandler, opts ...messaging.SubscribeOption) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[subject] = append(s.handlers[subject], handler)
	return nil
}

// Close is a no-op for the in-process stream. It exists to satisfy the
// messaging.Stream interface and allows callers to use a uniform lifecycle API.
func (s *InprocessStream) Close() error { return nil }

// inprocessMessage implements messaging.Message for in-process delivery.
// Ack, Nak, NakWithDelay, and Term are no-ops because there is no broker.
type inprocessMessage struct {
	id      string
	subject string
	data    []byte
	headers map[string]string
}

func (m *inprocessMessage) ID() string                      { return m.id }
func (m *inprocessMessage) Subject() string                 { return m.subject }
func (m *inprocessMessage) Data() []byte                    { return m.data }
func (m *inprocessMessage) Headers() map[string]string      { return m.headers }
func (m *inprocessMessage) Ack() error                      { return nil }
func (m *inprocessMessage) Nak() error                      { return nil }
func (m *inprocessMessage) NakWithDelay(_ time.Duration) error { return nil }
func (m *inprocessMessage) Term() error                     { return nil }
func (m *inprocessMessage) DeliveryAttempt() int            { return 1 }
