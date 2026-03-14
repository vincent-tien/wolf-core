// Package sync provides a no-op transport for explicitly sync-routed messages.
package sync

import (
	"context"
	"strings"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/transport"
)

// Transport is a no-op transport. Sync messages bypass transport entirely;
// this exists only for "sync://" DSN support in configuration.
type Transport struct{}

func (Transport) Name() string { return "sync" }
func (Transport) Close() error { return nil }

func (Transport) Send(_ context.Context, _ messenger.Envelope) error {
	return transport.ErrNotSupported
}

func (Transport) Get(_ context.Context) ([]messenger.Envelope, error) {
	return nil, transport.ErrNotSupported
}

func (Transport) Ack(_ context.Context, _ messenger.Envelope) error {
	return transport.ErrNotSupported
}

func (Transport) Reject(_ context.Context, _ messenger.Envelope, _ error) error {
	return transport.ErrNotSupported
}

// Factory creates sync transports from "sync://" DSN strings.
type Factory struct{}

func (Factory) Supports(dsn string) bool {
	return strings.HasPrefix(dsn, "sync://")
}

func (Factory) Create(_ string, _ map[string]any) (transport.Transport, error) {
	return Transport{}, nil
}
