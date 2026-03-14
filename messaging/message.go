// Package messaging defines broker-agnostic transport abstractions for the
// wolf-be platform. It sits in the shared kernel so every layer can depend on
// the contracts without pulling in any infrastructure dependency.
package messaging

import (
	"context"
	"time"
)

// Message represents a single message received from a broker or in-process
// stream. It carries raw bytes, headers, and delivery metadata.
// The Ack/Nak family of methods control the message lifecycle; in-process
// implementations provide no-op variants.
type Message interface {
	// ID returns the unique message identifier assigned by the broker.
	ID() string
	// Subject returns the topic/subject the message was published on.
	Subject() string
	// Data returns the raw serialised payload bytes.
	Data() []byte
	// Headers returns the key-value metadata attached to the message.
	Headers() map[string]string
	// Ack positively acknowledges the message, signalling successful processing.
	Ack() error
	// Nak negatively acknowledges the message, requesting redelivery.
	Nak() error
	// NakWithDelay negatively acknowledges the message with a redelivery delay.
	NakWithDelay(d time.Duration) error
	// Term terminates the message without redelivery (equivalent to discard).
	Term() error
	// DeliveryAttempt returns the 1-based delivery count for this message.
	DeliveryAttempt() int
}

// MessageHandler is a function that processes a single received message.
// It must return nil on success. A non-nil error signals the broker that the
// message should be redelivered (Nak semantics).
type MessageHandler func(ctx context.Context, msg Message) error
