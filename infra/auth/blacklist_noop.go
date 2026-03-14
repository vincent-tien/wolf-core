// blacklist_noop.go — No-op token blacklist for development (never revokes).
package auth

import (
	"context"
	"time"
)

// NoopBlacklist is a blacklist implementation that never revokes any token.
// It is intended for use in development environments or unit tests where
// a real cache is not available.
type NoopBlacklist struct{}

// NewNoopBlacklist returns a NoopBlacklist.
func NewNoopBlacklist() *NoopBlacklist {
	return &NoopBlacklist{}
}

// IsBlacklisted always returns false — no token is ever blacklisted.
func (n *NoopBlacklist) IsBlacklisted(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// Blacklist is a no-op — the JTI is silently discarded.
func (n *NoopBlacklist) Blacklist(_ context.Context, _ string, _ time.Time) error {
	return nil
}
