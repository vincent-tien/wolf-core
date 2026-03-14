// token.go — Token contracts consumed by auth middleware and IAM module.
//
// These interfaces define the boundary between platform (JWT implementation)
// and business logic (IAM login/refresh use cases). The platform implements
// TokenValidator and TokenBlacklist; middleware and modules consume them.
package auth

import (
	"context"
	"time"
)

// TokenType distinguishes access vs refresh tokens.
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// TokenPair holds both access and refresh tokens returned on login/refresh.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int64     `json:"expires_in"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// TokenValidator validates tokens and extracts claims.
// Implemented by platform/auth. Consumed by middleware.
type TokenValidator interface {
	// ValidateAccessToken validates an access token and returns claims.
	ValidateAccessToken(ctx context.Context, tokenString string) (*UserClaims, error)
}

// TokenBlacklist checks whether a token/session has been revoked.
type TokenBlacklist interface {
	// IsBlacklisted checks if a token or session has been revoked.
	IsBlacklisted(ctx context.Context, jti string) (bool, error)
	// Blacklist adds a token to the blacklist until its natural expiry.
	Blacklist(ctx context.Context, jti string, expiresAt time.Time) error
}

// SessionRevocationChecker provides a durable (DB-backed) fallback for session
// revocation checks. Used by token validation when the fast-path cache (Redis)
// reports "not blacklisted" — the DB is the source of truth for revocation.
type SessionRevocationChecker interface {
	IsSessionRevoked(ctx context.Context, sessionID string) (bool, error)
}
