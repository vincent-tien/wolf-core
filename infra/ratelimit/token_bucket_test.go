package ratelimit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/ratelimit"
)

func TestTokenBucket_AllowUnderLimit(t *testing.T) {
	t.Parallel()

	// Arrange: 5 rps, burst of 5.
	tb := ratelimit.NewTokenBucket(5, 5)
	defer tb.Close()

	const key = "user-1"

	// Act & Assert: all 5 requests within burst capacity must succeed.
	for i := range 5 {
		got := tb.Allow(key)
		require.True(t, got, "expected request %d to be allowed", i+1)
	}
}

func TestTokenBucket_BlockOverLimit(t *testing.T) {
	t.Parallel()

	// Arrange: 1 rps, burst of 1 — only the first request is allowed.
	tb := ratelimit.NewTokenBucket(1, 1)
	defer tb.Close()

	const key = "user-block"

	// Act: exhaust the single token.
	first := tb.Allow(key)
	second := tb.Allow(key)

	// Assert.
	assert.True(t, first, "first request should be allowed")
	assert.False(t, second, "second request should be blocked when burst is exhausted")
}

func TestTokenBucket_DifferentKeys(t *testing.T) {
	t.Parallel()

	// Arrange: burst of 1 — each key has its own independent limiter.
	tb := ratelimit.NewTokenBucket(1, 1)
	defer tb.Close()

	// Act.
	allowedA := tb.Allow("key-a")
	allowedB := tb.Allow("key-b")
	blockedA := tb.Allow("key-a")

	// Assert.
	assert.True(t, allowedA, "key-a first request should be allowed")
	assert.True(t, allowedB, "key-b first request should be allowed (independent limiter)")
	assert.False(t, blockedA, "key-a second request should be blocked")
}
