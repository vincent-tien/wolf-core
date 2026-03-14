// context.go — Context propagation for authenticated user claims.
//
// Auth middleware (platform/middleware/http) injects UserClaims into the
// request context via WithClaims(). All downstream layers retrieve claims
// via ClaimsFromContext() without importing any middleware package.
//
// This file lives in shared/auth (not platform/) so that domain and
// application layers can read claims without violating the dependency
// direction rule: shared ← platform ← modules.
package auth

import "context"

// contextKey is an unexported type to prevent collisions in context.
type contextKey int

const (
	claimsKey contextKey = iota
)

// WithClaims stores UserClaims in the context.
func WithClaims(ctx context.Context, claims *UserClaims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// ClaimsFromContext extracts UserClaims from the context.
// Returns nil if not authenticated (no claims present).
func ClaimsFromContext(ctx context.Context) *UserClaims {
	claims, _ := ctx.Value(claimsKey).(*UserClaims)
	return claims
}

// MustClaimsFromContext extracts UserClaims from the context.
// Panics if claims are not present -- use only AFTER auth middleware.
func MustClaimsFromContext(ctx context.Context) *UserClaims {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		panic("auth: claims not found in context -- missing auth middleware?")
	}
	return claims
}

// UserIDFromContext is a convenience helper that returns just the user ID.
// Returns empty string if not authenticated.
func UserIDFromContext(ctx context.Context) string {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	return claims.UserID
}
