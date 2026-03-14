// stream.go — Broker-agnostic pub/sub abstraction (Stream, Publisher, Subscriber, Message).
package messaging

import "context"

// RawMessage is the value type used when publishing to a stream.
// It contains everything a broker adapter needs to route and persist the
// message without any domain knowledge.
type RawMessage struct {
	// ID is an optional client-assigned idempotency key. Adapters may use it
	// as the broker message ID; leave empty to let the broker assign one.
	ID string
	// Name is a human-readable label for observability (e.g. event type).
	Name string
	// Subject is the topic/subject to publish on.
	Subject string
	// Data is the serialised payload bytes.
	Data []byte
	// Headers carries arbitrary key-value metadata (e.g. event type, trace IDs).
	Headers map[string]string
}

// Publisher is the write side of the messaging stream contract.
// Implementations must be safe for concurrent use.
type Publisher interface {
	// Publish sends msg on the given subject. The subject parameter takes
	// precedence over msg.Subject so callers can override routing at publish
	// time without mutating the message value.
	Publish(ctx context.Context, subject string, msg RawMessage) error
}

// Subscriber is the read side of the messaging stream contract.
// Implementations must be safe for concurrent use.
type Subscriber interface {
	// Subscribe registers handler for all messages arriving on subject.
	// Options configure consumer group, durability, and ack behaviour.
	// Multiple calls with the same subject append additional handlers.
	Subscribe(subject string, handler MessageHandler, opts ...SubscribeOption) error
}

// Stream combines Publisher, Subscriber, and lifecycle management into a single
// broker-agnostic facade. Callers depend on Stream; adapters implement it.
type Stream interface {
	Publisher
	Subscriber
	// Close drains in-flight messages and releases underlying resources.
	Close() error
}
