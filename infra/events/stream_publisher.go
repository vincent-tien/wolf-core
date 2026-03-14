// stream_publisher.go — Adapts messaging.Stream into event.Publisher for the outbox relay.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/vincent-tien/wolf-core/event"
	"github.com/vincent-tien/wolf-core/messaging"
)

// streamEventPublisher adapts a messaging.Stream into an event.Publisher.
// It serialises domain events into raw messages and publishes them using the
// event type as the subject. This is used by the outbox relay worker to bridge
// persisted outbox entries to the messaging infrastructure.
type streamEventPublisher struct {
	stream messaging.Stream
}

// NewStreamEventPublisher returns an event.Publisher that publishes events to
// the given messaging stream. The event type string is used as the subject.
func NewStreamEventPublisher(stream messaging.Stream) event.Publisher {
	return &streamEventPublisher{stream: stream}
}

// Publish serialises evt and sends it on a subject derived from the event type.
func (p *streamEventPublisher) Publish(ctx context.Context, evt event.Event) error {
	subject := evt.EventType()

	meta := evt.GetMetadata()
	metaMap := meta.ToMap()
	headers := make(map[string]string, 6+len(metaMap))
	headers["event_id"] = evt.EventID()
	headers["event_type"] = evt.EventType()
	headers["aggregate_id"] = evt.AggregateID()
	headers["aggregate_type"] = evt.AggregateType()
	headers["occurred_at"] = evt.OccurredAt().UTC().Format("2006-01-02T15:04:05.999999999Z")
	headers["version"] = strconv.Itoa(evt.Version())
	for k, v := range metaMap {
		headers[k] = v
	}

	var data []byte
	if evt.Payload() != nil {
		// The outbox stores events as pre-serialised JSON payloads, so we
		// marshal the payload directly via fmt. For the relay, payloads
		// arrive as map[string]interface{} after JSON round-trip; re-encoding
		// to JSON is safe.
		// If a TypeRegistry is needed for stricter encoding, the caller should
		// wrap this publisher with an EventStream instead.
		encoded, err := marshalPayload(evt.Payload())
		if err != nil {
			return fmt.Errorf("stream_publisher: encode payload for %q: %w", subject, err)
		}
		data = encoded
	} else {
		data = []byte("{}")
	}

	msg := messaging.RawMessage{
		ID:      evt.EventID(),
		Name:    evt.EventType(),
		Subject: subject,
		Data:    data,
		Headers: headers,
	}

	if err := p.stream.Publish(ctx, subject, msg); err != nil {
		return fmt.Errorf("stream_publisher: publish %q: %w", subject, err)
	}
	return nil
}

func marshalPayload(payload any) ([]byte, error) {
	if raw, ok := payload.([]byte); ok {
		return raw, nil
	}
	return json.Marshal(payload)
}
