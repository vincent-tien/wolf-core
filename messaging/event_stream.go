// event_stream.go — Typed event pub/sub over a raw messaging.Stream.
package messaging

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/vincent-tien/wolf-core/event"
)

// EventHandler is a function that processes a typed domain event received from
// a stream. The event is fully reconstructed (including metadata headers)
// before the handler is called.
type EventHandler func(ctx context.Context, evt event.Event) error

// EventStream wraps a Stream with typed serialization/deserialization using an
// event.TypeRegistry. It translates between raw broker messages ([]byte +
// headers) and domain event values, keeping transport details out of handlers.
type EventStream struct {
	stream   Stream
	registry *event.TypeRegistry
}

// NewEventStream creates an EventStream backed by stream, using registry to
// encode and decode event payloads.
func NewEventStream(stream Stream, registry *event.TypeRegistry) *EventStream {
	return &EventStream{stream: stream, registry: registry}
}

// Publish serialises evt.Payload() via the registry and sends it on subject.
// Event metadata (ID, type, aggregate info, schema version, timestamps,
// correlation/causation/trace IDs) are encoded as message headers so consumers
// can reconstruct the full event without deserialising the payload body.
func (es *EventStream) Publish(ctx context.Context, subject string, evt event.Event) error {
	var data []byte
	var err error

	if evt.Payload() != nil {
		data, err = es.registry.Serialize(evt.EventType(), evt.Payload())
		if err != nil {
			return fmt.Errorf("event_stream: serialize %q: %w", evt.EventType(), err)
		}
	} else {
		data = []byte("{}")
	}

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

	msg := RawMessage{
		ID:      evt.EventID(),
		Name:    evt.EventType(),
		Subject: subject,
		Data:    data,
		Headers: headers,
	}

	if err := es.stream.Publish(ctx, subject, msg); err != nil {
		return fmt.Errorf("event_stream: publish %q: %w", evt.EventType(), err)
	}
	return nil
}

// Subscribe registers handler on subject. Incoming messages are deserialised
// using the registry; the reconstructed event (with metadata from headers) is
// passed to handler. If deserialisation fails the message is Ack'd to avoid
// poison-pill redelivery loops; the error is returned from the MessageHandler
// so adapters can log it.
func (es *EventStream) Subscribe(subject string, handler EventHandler, opts ...SubscribeOption) error {
	msgHandler := func(ctx context.Context, msg Message) error {
		headers := msg.Headers()
		eventType := headers["event_type"]
		if eventType == "" {
			_ = msg.Ack() // cannot process without type — discard
			return fmt.Errorf("event_stream: missing event_type header on subject %q", subject)
		}

		payload, err := es.registry.Deserialize(eventType, msg.Data())
		if err != nil {
			_ = msg.Ack() // undeserializable — discard to avoid poison pill
			return fmt.Errorf("event_stream: deserialize %q: %w", eventType, err)
		}

		version := 1
		if v, parseErr := strconv.Atoi(headers["version"]); parseErr == nil {
			version = v
		}

		evtOpts := make([]event.EventOption, 0, 5)
		evtOpts = append(evtOpts,
			event.WithAggregateInfo(headers["aggregate_id"], headers["aggregate_type"]),
			event.WithMetadata(event.MetadataFromMap(headers)),
			event.WithVersion(version),
			event.WithID(headers["event_id"]),
		)
		if ts := headers["occurred_at"]; ts != "" {
			if t, parseErr := time.Parse(time.RFC3339Nano, ts); parseErr == nil {
				evtOpts = append(evtOpts, event.WithOccurredAt(t))
			}
		}

		evt := event.NewEvent(eventType, payload, evtOpts...)

		if err := handler(ctx, evt); err != nil {
			return err // let the adapter decide ack/nak
		}

		return msg.Ack()
	}

	return es.stream.Subscribe(subject, msgHandler, opts...)
}

