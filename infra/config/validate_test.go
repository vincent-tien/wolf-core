package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidateCORS_CredentialsWithWildcard_ReturnsError(t *testing.T) {
	cfg := CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
	}

	errs := validateCORS(cfg)

	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "wildcard")
}

func TestValidateCORS_CredentialsWithExplicitOrigins_NoError(t *testing.T) {
	cfg := CORSConfig{
		AllowedOrigins:   []string{"https://example.com"},
		AllowCredentials: true,
	}

	errs := validateCORS(cfg)

	assert.Empty(t, errs)
}

func TestValidateCORS_NegativeMaxAge_ReturnsError(t *testing.T) {
	cfg := CORSConfig{
		MaxAge: -1,
	}

	errs := validateCORS(cfg)

	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "max_age")
}

func TestValidateCORS_ValidConfig_NoError(t *testing.T) {
	cfg := CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
		MaxAge:         3600,
	}

	errs := validateCORS(cfg)

	assert.Empty(t, errs)
}

// ─── JWT Audience ────────────────────────────────────────────────────────────

func validJWTConfig() JWTConfig {
	return JWTConfig{
		SigningMethod:   "HS256",
		SecretKey:       "00000000000000000000000000000000",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 168 * time.Hour,
		Issuer:          "test",
		Audience:        []string{"test-aud"},
	}
}

func TestValidateJWT_ExactlyOneAudience_NoError(t *testing.T) {
	errs := validateJWT(validJWTConfig())

	assert.Empty(t, errs)
}

func TestValidateJWT_ZeroAudience_ReturnsError(t *testing.T) {
	cfg := validJWTConfig()
	cfg.Audience = nil

	errs := validateJWT(cfg)

	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[len(errs)-1].Error(), "exactly one entry")
}

func TestValidateJWT_MultipleAudiences_ReturnsError(t *testing.T) {
	cfg := validJWTConfig()
	cfg.Audience = []string{"aud-1", "aud-2"}

	errs := validateJWT(cfg)

	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[len(errs)-1].Error(), "exactly one entry")
	assert.Contains(t, errs[len(errs)-1].Error(), "got 2")
}

// ─── Production Safety: Trusted Proxies ──────────────────────────────────────

func TestValidateProductionSafety_RPSWithoutTrustedProxies_ReturnsError(t *testing.T) {
	cfg := &Config{
		App:       AppConfig{Env: EnvProduction},
		Broker:    BrokerConfig{Driver: BrokerNATS},
		Cache:     CacheConfig{Driver: CacheDriverRedis},
		JWT:       JWTConfig{SecretKey: "production-safe-secret-key-here!"},
		RateLimit: RateLimitConfig{RPS: 100},
		HTTP:      HTTPConfig{TrustedProxies: nil},
	}

	errs := validateProductionSafety(cfg)

	var found bool
	for _, e := range errs {
		if strings.Contains(e.Error(), "trusted_proxies") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected trusted_proxies error in production with RPS>0")
}

func TestValidateProductionSafety_RPSWithTrustedProxies_NoProxyError(t *testing.T) {
	cfg := &Config{
		App:       AppConfig{Env: EnvProduction},
		Broker:    BrokerConfig{Driver: BrokerNATS},
		Cache:     CacheConfig{Driver: CacheDriverRedis},
		JWT:       JWTConfig{SecretKey: "production-safe-secret-key-here!"},
		RateLimit: RateLimitConfig{RPS: 100},
		HTTP:      HTTPConfig{TrustedProxies: []string{"10.0.0.0/8"}},
	}

	errs := validateProductionSafety(cfg)

	for _, e := range errs {
		assert.NotContains(t, e.Error(), "trusted_proxies")
	}
}
