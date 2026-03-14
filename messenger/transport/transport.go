// Package transport defines the interfaces for message transport backends.
package transport

import (
	"context"
	"errors"

	"github.com/vincent-tien/wolf-core/messenger"
)

// ErrNotSupported is returned by transports that don't support an operation.
var ErrNotSupported = errors.New("transport: operation not supported")

// Sender sends envelopes to an external system.
type Sender interface {
	Send(ctx context.Context, env messenger.Envelope) error
}

// Receiver pulls envelopes from an external system.
type Receiver interface {
	Get(ctx context.Context) ([]messenger.Envelope, error)
	Ack(ctx context.Context, env messenger.Envelope) error
	Reject(ctx context.Context, env messenger.Envelope, reason error) error
}

// Transport combines Sender + Receiver + lifecycle.
type Transport interface {
	Sender
	Receiver
	Name() string
	Close() error
}
