// blacklist_resilient.go — Circuit-breaker wrapped blacklist for production resilience.
//
// Wraps any TokenBlacklist with a circuit breaker. On IsBlacklisted failures:
// fail-open (return false) so auth continues working during Redis outages.
// Blacklist writes propagate errors normally — revocation failure should be visible.
package auth

import (
	"context"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"

	sharedauth "github.com/vincent-tien/wolf-core/auth"
)

// ResilientBlacklist wraps a TokenBlacklist with circuit breaker protection.
// IsBlacklisted uses fail-open: if the circuit is open or the inner call fails,
// the token is treated as NOT blacklisted. This trades a bounded window of
// accepting revoked tokens (capped by token TTL) for API availability.
type ResilientBlacklist struct {
	inner  sharedauth.TokenBlacklist
	cb     *gobreaker.CircuitBreaker
	logger *zap.Logger
}

// NewResilientBlacklist wraps inner with a pre-built circuit breaker.
// Use resilience.NewCircuitBreaker to construct cb for consistent metrics
// and tripping strategy across the codebase.
func NewResilientBlacklist(
	inner sharedauth.TokenBlacklist,
	cb *gobreaker.CircuitBreaker,
	logger *zap.Logger,
) *ResilientBlacklist {
	return &ResilientBlacklist{inner: inner, cb: cb, logger: logger}
}

// IsBlacklisted checks the blacklist through the circuit breaker.
// On ANY failure (circuit open, Redis error), returns false (fail-open).
// Logging the first 8 chars of the JTI (UUID prefix) is safe for debugging
// without leaking the full token identifier.
func (r *ResilientBlacklist) IsBlacklisted(ctx context.Context, jti string) (bool, error) {
	result, err := r.cb.Execute(func() (any, error) {
		return r.inner.IsBlacklisted(ctx, jti)
	})
	if err != nil {
		r.logger.Warn("blacklist check failed, fail-open",
			zap.String("jti_prefix", jti[:min(8, len(jti))]),
			zap.Error(err),
		)
		return false, nil
	}
	blacklisted, ok := result.(bool)
	if !ok {
		return false, nil
	}
	return blacklisted, nil
}

// Blacklist delegates to the inner implementation without circuit breaker.
// Revocation errors should propagate so callers know the operation failed.
func (r *ResilientBlacklist) Blacklist(ctx context.Context, jti string, expiresAt time.Time) error {
	return r.inner.Blacklist(ctx, jti, expiresAt)
}
