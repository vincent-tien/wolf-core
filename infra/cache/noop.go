// Package cache provides a pluggable caching abstraction for the wolf-be service.
package cache

import (
	"context"
	"time"
)

// noopClient is a no-operation implementation of Client. All write operations
// succeed silently and Get always returns ErrCacheMiss. It is useful in tests
// and in environments where caching is intentionally disabled.
type noopClient struct{}

// NewNoopClient returns a Client whose operations are all no-ops. Get always
// returns ErrCacheMiss; Set, Delete, Ping, and Close always return nil.
func NewNoopClient() Client {
	return &noopClient{}
}

// Get always returns ErrCacheMiss because the noop cache stores nothing.
func (n *noopClient) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, ErrCacheMiss
}

// Set silently discards the value.
func (n *noopClient) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

// Delete is a no-op.
func (n *noopClient) Delete(_ context.Context, _ ...string) error {
	return nil
}

// Ping always returns nil, indicating the noop backend is "healthy".
func (n *noopClient) Ping(_ context.Context) error {
	return nil
}

// Close is a no-op.
func (n *noopClient) Close() error {
	return nil
}
