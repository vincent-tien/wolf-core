// Package events provides factory functions and middleware for the messaging
// infrastructure.
package events

import (
	"context"

	"go.opentelemetry.io/otel"

	"github.com/vincent-tien/wolf-core/messaging"
)

// headerCarrier adapts a map[string]string to the propagation.TextMapCarrier
// interface so that OTel propagators can inject/extract trace context directly
// into/from message headers.
type headerCarrier map[string]string

func (c headerCarrier) Get(key string) string      { return c[key] }
func (c headerCarrier) Set(key, value string)       { c[key] = value }
func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// TracingPublisher wraps a messaging.Publisher and injects W3C trace context
// into every published message's headers, enabling distributed tracing across
// asynchronous boundaries.
type TracingPublisher struct {
	next messaging.Publisher
}

// NewTracingPublisher wraps next so that every Publish call injects the current
// OTel trace context into the message headers.
func NewTracingPublisher(next messaging.Publisher) *TracingPublisher {
	return &TracingPublisher{next: next}
}

// Publish injects trace context from ctx into msg.Headers and delegates to the
// wrapped publisher. If msg.Headers is nil, a new map is created.
func (p *TracingPublisher) Publish(ctx context.Context, subject string, msg messaging.RawMessage) error {
	if msg.Headers == nil {
		msg.Headers = make(map[string]string)
	}
	otel.GetTextMapPropagator().Inject(ctx, headerCarrier(msg.Headers))
	return p.next.Publish(ctx, subject, msg)
}

// TracingSubscribeMiddleware returns a MessageHandler that extracts W3C trace
// context from the message headers and starts a child context before calling
// the next handler. This allows downstream operations to participate in the
// same distributed trace as the publisher.
func TracingSubscribeMiddleware(next messaging.MessageHandler) messaging.MessageHandler {
	return func(ctx context.Context, msg messaging.Message) error {
		prop := otel.GetTextMapPropagator()
		ctx = prop.Extract(ctx, headerCarrier(msg.Headers()))
		return next(ctx, msg)
	}
}
