package auth_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/vincent-tien/wolf-core/infra/auth"
	"github.com/vincent-tien/wolf-core/infra/config"
	sharedauth "github.com/vincent-tien/wolf-core/auth"
	"github.com/vincent-tien/wolf-core/clock"
	sharederrors "github.com/vincent-tien/wolf-core/errors"
)

// ─── In-memory test blacklist ─────────────────────────────────────────────────

// memBlacklist is a thread-safe in-memory blacklist for testing.
type memBlacklist struct {
	mu      sync.RWMutex
	entries map[string]time.Time
}

func newMemBlacklist() *memBlacklist {
	return &memBlacklist{entries: make(map[string]time.Time)}
}

func (m *memBlacklist) IsBlacklisted(_ context.Context, jti string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.entries[jti]
	return ok, nil
}

func (m *memBlacklist) Blacklist(_ context.Context, jti string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[jti] = expiresAt
	return nil
}

// alwaysBlacklistedBL reports every token as revoked — used to test the
// blacklist-check code path in ValidateAccessToken.
type alwaysBlacklistedBL struct{}

func (a *alwaysBlacklistedBL) IsBlacklisted(_ context.Context, _ string) (bool, error) {
	return true, nil
}
func (a *alwaysBlacklistedBL) Blacklist(_ context.Context, _ string, _ time.Time) error {
	return nil
}

// Compile-time interface assertions.
var _ sharedauth.TokenBlacklist = (*memBlacklist)(nil)
var _ sharedauth.TokenBlacklist = (*alwaysBlacklistedBL)(nil)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// secret32 is a 32-character HMAC secret valid for HS256.
const secret32 = "00000000000000000000000000000000"

// differentSecret32 is a different secret to test signature rejection.
const differentSecret32 = "11111111111111111111111111111111"

func newHS256Config(secret string) config.JWTConfig {
	return config.JWTConfig{
		SigningMethod:   "HS256",
		SecretKey:       secret,
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 168 * time.Hour,
		Issuer:          "test-issuer",
		Audience:        []string{"test-audience"},
	}
}

func defaultParams() platformauth.TokenParams {
	return platformauth.TokenParams{
		UserID:      "user-123",
		Email:       "alice@example.com",
		Roles:       []string{"admin"},
		Permissions: []string{"orders:read", "orders:write"},
		SessionID:   "session-abc",
	}
}

// mutableClock is a FakeClock whose time can be advanced during a test.
type mutableClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *mutableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *mutableClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestGenerateTokenPair_Success(t *testing.T) {
	// Arrange
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)

	// Act
	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())

	// Assert
	require.NoError(t, err)
	assert.NotEmpty(t, pair.AccessToken)
	assert.NotEmpty(t, pair.RefreshToken)
	assert.NotEqual(t, pair.AccessToken, pair.RefreshToken)
	assert.Equal(t, "Bearer", pair.TokenType)
	assert.Equal(t, int64(15*60), pair.ExpiresIn)
	assert.Equal(t, fixedNow.Add(15*time.Minute), pair.ExpiresAt)
}

func TestValidateAccessToken_Valid(t *testing.T) {
	// Arrange
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)

	params := defaultParams()
	pair, err := svc.GenerateTokenPair(context.Background(), params)
	require.NoError(t, err)

	// Act
	claims, err := svc.ValidateAccessToken(context.Background(), pair.AccessToken)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, params.UserID, claims.UserID)
	assert.Equal(t, params.Email, claims.Email)
	assert.Equal(t, params.Roles, claims.Roles)
	assert.Equal(t, params.Permissions, claims.Permissions)
	assert.Equal(t, params.SessionID, claims.SessionID)
	assert.WithinDuration(t, fixedNow.Add(15*time.Minute), claims.ExpiresAt, time.Second)
}

func TestValidateAccessToken_Expired(t *testing.T) {
	// Arrange — issue token, then advance clock past access TTL.
	issueTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := &mutableClock{t: issueTime}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	// Advance clock 16 minutes — one minute past the 15-minute access TTL.
	clk.advance(16 * time.Minute)

	// Act
	_, err = svc.ValidateAccessToken(context.Background(), pair.AccessToken)

	// Assert
	require.Error(t, err)
	var appErr *sharederrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, sharederrors.ErrUnauthorized, appErr.Code)
	assert.Contains(t, appErr.Message, "expired")
}

func TestValidateAccessToken_WrongSignature(t *testing.T) {
	// Arrange — sign with one secret, validate with a different secret.
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}

	signerSvc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)
	verifierSvc, err := platformauth.NewJWTService(newHS256Config(differentSecret32), clk, newMemBlacklist())
	require.NoError(t, err)

	pair, err := signerSvc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	// Act — validate token signed by signerSvc using verifierSvc.
	_, err = verifierSvc.ValidateAccessToken(context.Background(), pair.AccessToken)

	// Assert
	require.Error(t, err)
	var appErr *sharederrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, sharederrors.ErrUnauthorized, appErr.Code)
}

func TestValidateAccessToken_WrongType(t *testing.T) {
	// Arrange — generate a refresh token and pass it to ValidateAccessToken.
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	// Act — pass the refresh token to the access-token validator.
	_, err = svc.ValidateAccessToken(context.Background(), pair.RefreshToken)

	// Assert
	require.Error(t, err)
	var appErr *sharederrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, sharederrors.ErrUnauthorized, appErr.Code)
}

func TestValidateAccessToken_Blacklisted(t *testing.T) {
	// Arrange — use a blacklist that always returns true for every JTI.
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, &alwaysBlacklistedBL{})
	require.NoError(t, err)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	// Act
	_, err = svc.ValidateAccessToken(context.Background(), pair.AccessToken)

	// Assert
	require.Error(t, err)
	var appErr *sharederrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, sharederrors.ErrUnauthorized, appErr.Code)
	assert.Contains(t, appErr.Message, "revoked")
}

func TestValidateRefreshToken_Valid(t *testing.T) {
	// Arrange
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)

	params := defaultParams()
	pair, err := svc.GenerateTokenPair(context.Background(), params)
	require.NoError(t, err)

	// Act
	rc, err := svc.ValidateRefreshToken(context.Background(), pair.RefreshToken)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, params.UserID, rc.UserID)
	assert.Equal(t, params.SessionID, rc.SessionID)
	assert.NotEmpty(t, rc.JTI)
	assert.WithinDuration(t, fixedNow.Add(168*time.Hour), rc.ExpiresAt, time.Second)
}

func TestValidateRefreshToken_Expired(t *testing.T) {
	// Arrange — issue token then advance clock past refresh TTL.
	issueTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := &mutableClock{t: issueTime}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	// Advance clock past refresh TTL (168h + 1h = 169h).
	clk.advance(169 * time.Hour)

	// Act
	_, err = svc.ValidateRefreshToken(context.Background(), pair.RefreshToken)

	// Assert
	require.Error(t, err)
	var appErr *sharederrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, sharederrors.ErrUnauthorized, appErr.Code)
	assert.Contains(t, appErr.Message, "expired")
}

func TestRevokeToken_ThenValidate_Revoked(t *testing.T) {
	// Arrange
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	bl := newMemBlacklist()
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, bl)
	require.NoError(t, err)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	// Extract refresh token JTI.
	rc, err := svc.ValidateRefreshToken(context.Background(), pair.RefreshToken)
	require.NoError(t, err)

	// Act — revoke the refresh token by its JTI.
	err = svc.RevokeToken(context.Background(), rc.JTI, rc.ExpiresAt)
	require.NoError(t, err)

	// Assert — the JTI is stored in the blacklist.
	blacklisted, err := bl.IsBlacklisted(context.Background(), rc.JTI)
	require.NoError(t, err)
	assert.True(t, blacklisted, "JTI must be present in the blacklist after RevokeToken")
}

func TestNewJWTService_RejectsZeroAudience(t *testing.T) {
	cfg := newHS256Config(secret32)
	cfg.Audience = nil

	_, err := platformauth.NewJWTService(cfg, clock.FakeClock{}, newMemBlacklist())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one audience")
}

func TestNewJWTService_RejectsMultipleAudiences(t *testing.T) {
	cfg := newHS256Config(secret32)
	cfg.Audience = []string{"aud-1", "aud-2"}

	_, err := platformauth.NewJWTService(cfg, clock.FakeClock{}, newMemBlacklist())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one audience")
	assert.Contains(t, err.Error(), "got 2")
}

func TestValidateAccessToken_WrongAudience(t *testing.T) {
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}

	// Sign with audience "aud-a".
	cfgA := newHS256Config(secret32)
	cfgA.Audience = []string{"aud-a"}
	svcA, err := platformauth.NewJWTService(cfgA, clk, newMemBlacklist())
	require.NoError(t, err)

	pair, err := svcA.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	// Validate with audience "aud-b" — must reject.
	cfgB := newHS256Config(secret32)
	cfgB.Audience = []string{"aud-b"}
	svcB, err := platformauth.NewJWTService(cfgB, clk, newMemBlacklist())
	require.NoError(t, err)

	_, err = svcB.ValidateAccessToken(context.Background(), pair.AccessToken)

	require.Error(t, err)
	var appErr *sharederrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, sharederrors.ErrUnauthorized, appErr.Code)
}

// ─── Session revocation checker stub ────────────────────────────────────────

type stubSessionChecker struct {
	revoked bool
	err     error
	calls   int
}

func (s *stubSessionChecker) IsSessionRevoked(_ context.Context, _ string) (bool, error) {
	s.calls++
	return s.revoked, s.err
}

var _ sharedauth.SessionRevocationChecker = (*stubSessionChecker)(nil)

// ─── DB Fallback Tests ──────────────────────────────────────────────────────

func TestValidateAccessToken_DBFallback_Revoked(t *testing.T) {
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	bl := newMemBlacklist()
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, bl)
	require.NoError(t, err)

	checker := &stubSessionChecker{revoked: true}
	svc.SetSessionRevocationChecker(checker)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	_, err = svc.ValidateAccessToken(context.Background(), pair.AccessToken)

	require.Error(t, err)
	var ae *sharederrors.AppError
	require.ErrorAs(t, err, &ae)
	assert.Contains(t, ae.Message, "revoked")
	assert.Equal(t, 1, checker.calls)

	// Redis was backfilled.
	blacklisted, _ := bl.IsBlacklisted(context.Background(), "session:session-abc")
	assert.True(t, blacklisted, "Redis should be backfilled after DB-confirmed revocation")
}

func TestValidateAccessToken_DBFallback_NotRevoked(t *testing.T) {
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)

	checker := &stubSessionChecker{revoked: false}
	svc.SetSessionRevocationChecker(checker)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	claims, err := svc.ValidateAccessToken(context.Background(), pair.AccessToken)

	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, 1, checker.calls)
}

func TestValidateAccessToken_DBFallback_Error(t *testing.T) {
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)

	checker := &stubSessionChecker{err: fmt.Errorf("connection refused")}
	svc.SetSessionRevocationChecker(checker)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	_, err = svc.ValidateAccessToken(context.Background(), pair.AccessToken)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session revocation db check")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestValidateAccessToken_DBFallback_NegativeCacheSkipsDB(t *testing.T) {
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)

	checker := &stubSessionChecker{revoked: false}
	svc.SetSessionRevocationChecker(checker)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	// First call — hits DB, populates negative cache.
	_, err = svc.ValidateAccessToken(context.Background(), pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, 1, checker.calls)

	// Second call — negative cache hit, DB skipped.
	_, err = svc.ValidateAccessToken(context.Background(), pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, 1, checker.calls, "DB should not be called while negative cache is valid")
}

func TestValidateAccessToken_DBFallback_NegativeCacheExpires(t *testing.T) {
	issueTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := &mutableClock{t: issueTime}
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, newMemBlacklist())
	require.NoError(t, err)

	checker := &stubSessionChecker{revoked: false}
	svc.SetSessionRevocationChecker(checker)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	// First call — hits DB.
	_, err = svc.ValidateAccessToken(context.Background(), pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, 1, checker.calls)

	// Advance clock past the 30s negative cache TTL.
	clk.advance(31 * time.Second)

	// Second call — cache expired, DB queried again.
	_, err = svc.ValidateAccessToken(context.Background(), pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, 2, checker.calls, "DB should be called again after negative cache expires")
}

func TestRevokeSession_InvalidatesNegativeCache(t *testing.T) {
	fixedNow := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	clk := clock.FakeClock{Fixed: fixedNow}
	bl := newMemBlacklist()
	svc, err := platformauth.NewJWTService(newHS256Config(secret32), clk, bl)
	require.NoError(t, err)

	checker := &stubSessionChecker{revoked: false}
	svc.SetSessionRevocationChecker(checker)

	pair, err := svc.GenerateTokenPair(context.Background(), defaultParams())
	require.NoError(t, err)

	// Populate negative cache.
	_, err = svc.ValidateAccessToken(context.Background(), pair.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, 1, checker.calls)

	// Revoke — invalidates negative cache + writes Redis key.
	err = svc.RevokeSession(context.Background(), "session-abc", fixedNow.Add(15*time.Minute))
	require.NoError(t, err)

	// Next validate hits Redis blacklist (now populated), not negative cache.
	_, err = svc.ValidateAccessToken(context.Background(), pair.AccessToken)
	require.Error(t, err)
	var ae *sharederrors.AppError
	require.ErrorAs(t, err, &ae)
	assert.Contains(t, ae.Message, "revoked")
	assert.Equal(t, 1, checker.calls, "DB not called — Redis catches it first")
}
