// Package auth provides platform-level JWT token generation and validation,
// token blacklisting, and password hashing for the wolf-be service.
package auth

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/vincent-tien/wolf-core/infra/config"
	sharedauth "github.com/vincent-tien/wolf-core/auth"
	"github.com/vincent-tien/wolf-core/clock"
	sharederrors "github.com/vincent-tien/wolf-core/errors"
)

// sessionBlacklistPrefix is the key prefix used in the blacklist store to
// distinguish session-level revocation entries from individual token (JTI) entries.
const sessionBlacklistPrefix = "session:"

// sessionActiveCacheTTL is the duration an in-process negative-cache entry
// lives. While cached, the DB fallback is skipped for that session. A genuine
// revocation that misses Redis is detected within this TTL on the same pod;
// other pods without the cache entry detect it immediately.
const sessionActiveCacheTTL = 30 * time.Second

// customClaims extends the standard JWT registered claims with application-specific
// fields embedded in every token.
type customClaims struct {
	jwt.RegisteredClaims

	// Email is the authenticated user's email address.
	Email string `json:"email"`
	// Roles is the list of roles assigned to the user at token issuance.
	Roles []string `json:"roles"`
	// Permissions is the list of explicit permissions granted to the user.
	Permissions []string `json:"permissions"`
	// SessionID links the token to a specific device/session for revocation.
	SessionID string `json:"sid"`
	// TokenType distinguishes access tokens from refresh tokens.
	TokenType string `json:"type"`
}

// TokenParams contains the input data required to generate a token pair.
type TokenParams struct {
	// UserID is the unique identifier of the authenticated user.
	UserID string
	// Email is the user's email address.
	Email string
	// Roles is the list of roles to embed in the access token.
	Roles []string
	// Permissions is the list of permissions to embed in the access token.
	Permissions []string
	// SessionID uniquely identifies the login session (device).
	SessionID string
}

// RefreshClaims contains the data extracted from a validated refresh token.
type RefreshClaims struct {
	// UserID is the subject of the refresh token.
	UserID string
	// SessionID is the session the refresh token belongs to.
	SessionID string
	// JTI is the unique token identifier, used for blacklisting.
	JTI string
	// ExpiresAt is when the refresh token expires.
	ExpiresAt time.Time
}

// JWTService generates and validates JWT tokens using either HS256 or RS256.
// It is safe for concurrent use from multiple goroutines.
type JWTService struct {
	cfg            config.JWTConfig
	clk            clock.Clock
	blacklist      sharedauth.TokenBlacklist
	sessionChecker sharedauth.SessionRevocationChecker
	activeCache    sync.Map // sessionID → time.Time (expiry); negative cache for DB fallback
	signingKey     interface{}
	verifyKey      interface{}
	method         jwt.SigningMethod
}

// NewJWTService constructs a JWTService from the provided configuration.
// For HS256 the secret key is taken directly from cfg.SecretKey.
// For RS256 the PEM files at cfg.PrivateKeyPath and cfg.PublicKeyPath are loaded.
func NewJWTService(cfg config.JWTConfig, clk clock.Clock, bl sharedauth.TokenBlacklist) (*JWTService, error) {
	if len(cfg.Audience) != 1 {
		return nil, fmt.Errorf("jwt: exactly one audience must be configured, got %d", len(cfg.Audience))
	}

	svc := &JWTService{
		cfg:       cfg,
		clk:       clk,
		blacklist: bl,
	}

	switch cfg.SigningMethod {
	case "HS256":
		svc.method = jwt.SigningMethodHS256
		key := []byte(cfg.SecretKey)
		svc.signingKey = key
		svc.verifyKey = key

	case "RS256":
		privateKey, err := loadRSAPrivateKey(cfg.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("jwt: load private key: %w", err)
		}
		publicKey, err := loadRSAPublicKey(cfg.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("jwt: load public key: %w", err)
		}
		svc.method = jwt.SigningMethodRS256
		svc.signingKey = privateKey
		svc.verifyKey = publicKey

	default:
		return nil, fmt.Errorf("jwt: unsupported signing method %q", cfg.SigningMethod)
	}

	return svc, nil
}

// SetSessionRevocationChecker attaches an optional durable (DB-backed) fallback
// for session revocation. When set, ValidateAccessToken consults the DB if the
// fast-path Redis blacklist reports "not revoked", closing the window where a
// post-commit Redis failure leaves revoked sessions active.
//
// Called after module registration because the checker is owned by a module
// (IAM repo) that is constructed after the platform JWTService.
func (s *JWTService) SetSessionRevocationChecker(c sharedauth.SessionRevocationChecker) {
	s.sessionChecker = c
}

// GenerateTokenPair creates a new access and refresh token pair for the given user.
// The access token embeds all claims including roles and permissions.
// The refresh token only embeds the user ID, session ID, and a unique JTI.
func (s *JWTService) GenerateTokenPair(ctx context.Context, params TokenParams) (*sharedauth.TokenPair, error) {
	now := s.clk.Now()
	accessExpiry := now.Add(s.cfg.AccessTokenTTL)
	refreshExpiry := now.Add(s.cfg.RefreshTokenTTL)

	accessJTI := uuid.NewString()
	refreshJTI := uuid.NewString()

	// Build access token.
	accessClaims := customClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.cfg.Issuer,
			Subject:   params.UserID,
			Audience:  jwt.ClaimStrings(s.cfg.Audience),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			NotBefore: jwt.NewNumericDate(now),
			ID:        accessJTI,
		},
		Email:       params.Email,
		Roles:       params.Roles,
		Permissions: params.Permissions,
		SessionID:   params.SessionID,
		TokenType:   string(sharedauth.TokenTypeAccess),
	}

	accessToken, err := s.sign(accessClaims)
	if err != nil {
		return nil, fmt.Errorf("jwt: sign access token: %w", err)
	}

	// Build refresh token — minimal claims, no roles/permissions.
	refreshClaims := customClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.cfg.Issuer,
			Subject:   params.UserID,
			Audience:  jwt.ClaimStrings(s.cfg.Audience),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(refreshExpiry),
			NotBefore: jwt.NewNumericDate(now),
			ID:        refreshJTI,
		},
		SessionID: params.SessionID,
		TokenType: string(sharedauth.TokenTypeRefresh),
	}

	refreshToken, err := s.sign(refreshClaims)
	if err != nil {
		return nil, fmt.Errorf("jwt: sign refresh token: %w", err)
	}

	return &sharedauth.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.cfg.AccessTokenTTL.Seconds()),
		ExpiresAt:    accessExpiry,
	}, nil
}

// ValidateAccessToken parses and validates an access token string.
// It checks the signing method, token type, expiry, and blacklist status.
// Implements sharedauth.TokenValidator.
func (s *JWTService) ValidateAccessToken(ctx context.Context, tokenString string) (*sharedauth.UserClaims, error) {
	claims, err := s.parse(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != string(sharedauth.TokenTypeAccess) {
		return nil, sharederrors.NewTokenInvalid("wrong token type")
	}

	blacklisted, err := s.blacklist.IsBlacklisted(ctx, claims.ID)
	if err != nil {
		return nil, fmt.Errorf("jwt: blacklist check: %w", err)
	}
	if blacklisted {
		return nil, sharederrors.NewTokenRevoked()
	}

	// Check session-level revocation (logout, force-logout, password change).
	if claims.SessionID != "" {
		if err := s.checkSessionRevocation(ctx, claims); err != nil {
			return nil, err
		}
	}

	issuedAt := time.Time{}
	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.Time
	}
	expiresAt := time.Time{}
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}

	return sharedauth.NewUserClaims(
		claims.Subject,
		claims.Email,
		claims.Roles,
		claims.Permissions,
		claims.SessionID,
		issuedAt,
		expiresAt,
	), nil
}

// checkSessionRevocation checks whether a session has been revoked via Redis
// (fast path) or DB (durable fallback). An in-process negative cache avoids
// a DB round-trip on every request for active sessions.
func (s *JWTService) checkSessionRevocation(ctx context.Context, claims *customClaims) error {
	sessionKey := sessionBlacklistPrefix + claims.SessionID

	// Fast path: Redis blacklist.
	blacklisted, err := s.blacklist.IsBlacklisted(ctx, sessionKey)
	if err != nil {
		return fmt.Errorf("jwt: session blacklist check: %w", err)
	}
	if blacklisted {
		return sharederrors.NewTokenRevoked()
	}

	// DB fallback: if Redis says "not revoked", the DB is the source of truth.
	if s.sessionChecker == nil {
		return nil
	}

	now := s.clk.Now()

	// In-process negative cache: skip DB if we recently confirmed "not revoked".
	if exp, ok := s.activeCache.Load(claims.SessionID); ok {
		expiry, valid := exp.(time.Time)
		if valid && now.Before(expiry) {
			return nil
		}
		s.activeCache.Delete(claims.SessionID)
	}

	revoked, err := s.sessionChecker.IsSessionRevoked(ctx, claims.SessionID)
	if err != nil {
		return fmt.Errorf("jwt: session revocation db check: %w", err)
	}
	if !revoked {
		s.activeCache.Store(claims.SessionID, now.Add(sessionActiveCacheTTL))
		return nil
	}

	// Best-effort backfill: write revocation to Redis so the fast path catches
	// subsequent requests. Non-fatal — the DB check already gates this request.
	// TODO: log backfill failures once JWTService has a structured logger.
	if claims.ExpiresAt != nil {
		_ = s.blacklist.Blacklist(ctx, sessionKey, claims.ExpiresAt.Time) //nolint:errcheck // best-effort backfill
	}
	return sharederrors.NewTokenRevoked()
}

// ValidateRefreshToken parses and validates a refresh token string.
// It checks the signing method, token type, and expiry, but does NOT check
// the blacklist — that responsibility belongs to the refresh command handler.
func (s *JWTService) ValidateRefreshToken(ctx context.Context, tokenString string) (*RefreshClaims, error) {
	claims, err := s.parse(tokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != string(sharedauth.TokenTypeRefresh) {
		return nil, sharederrors.NewTokenInvalid("wrong token type")
	}

	expiresAt := time.Time{}
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}

	return &RefreshClaims{
		UserID:    claims.Subject,
		SessionID: claims.SessionID,
		JTI:       claims.ID,
		ExpiresAt: expiresAt,
	}, nil
}

// RevokeToken adds a specific token (by JTI) to the blacklist until its expiry.
func (s *JWTService) RevokeToken(ctx context.Context, jti string, expiresAt time.Time) error {
	return s.blacklist.Blacklist(ctx, jti, expiresAt)
}

// RevokeSession adds a session-level blacklist entry, preventing any token
// that carries the given session ID from being accepted.
// The key stored is "session:{sessionID}".
func (s *JWTService) RevokeSession(ctx context.Context, sessionID string, expiresAt time.Time) error {
	s.activeCache.Delete(sessionID)
	key := sessionBlacklistPrefix + sessionID
	return s.blacklist.Blacklist(ctx, key, expiresAt)
}

// sign creates a signed token string from the given claims.
func (s *JWTService) sign(claims customClaims) (string, error) {
	token := jwt.NewWithClaims(s.method, claims)
	return token.SignedString(s.signingKey)
}

// parse validates the token signature and expiry, returning the embedded claims.
// It maps jwt library errors to application error types.
func (s *JWTService) parse(tokenString string) (*customClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&customClaims{},
		func(t *jwt.Token) (interface{}, error) {
			return s.verifyKey, nil
		},
		jwt.WithValidMethods([]string{s.cfg.SigningMethod}),
		jwt.WithIssuer(s.cfg.Issuer),
		jwt.WithAudience(s.cfg.Audience[0]),
		jwt.WithLeeway(5*time.Second),
		jwt.WithTimeFunc(s.clk.Now),
	)
	if err != nil {
		return nil, mapJWTError(err)
	}

	claims, ok := token.Claims.(*customClaims)
	if !ok || !token.Valid {
		return nil, sharederrors.NewTokenInvalid("claims extraction failed")
	}

	return claims, nil
}

// mapJWTError converts errors from the golang-jwt library into application errors.
func mapJWTError(err error) error {
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return sharederrors.NewTokenExpired()
	case errors.Is(err, jwt.ErrTokenMalformed):
		return sharederrors.NewTokenInvalid("malformed")
	case errors.Is(err, jwt.ErrSignatureInvalid):
		return sharederrors.NewTokenInvalid("invalid signature")
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		return sharederrors.NewTokenInvalid("token not yet valid")
	default:
		return sharederrors.NewTokenInvalid(err.Error())
	}
}

// loadRSAPrivateKey reads a PEM-encoded RSA private key from disk.
func loadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key file %q: %w", path, err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}
	return key, nil
}

// loadRSAPublicKey reads a PEM-encoded RSA public key from disk.
func loadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key file %q: %w", path, err)
	}
	key, err := jwt.ParseRSAPublicKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse RSA public key: %w", err)
	}
	return key, nil
}
