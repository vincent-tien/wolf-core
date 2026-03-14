package ratelimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/ratelimit"
)

func TestFixedWindow_AllowUnderLimit(t *testing.T) {
	t.Parallel()

	// Arrange: 5 requests per second window.
	fw := ratelimit.NewFixedWindow(5, time.Second)

	const key = "fw-user-1"

	// Act & Assert: all 5 requests within the same window must be allowed.
	for i := range 5 {
		got := fw.Allow(key)
		require.True(t, got, "expected request %d to be allowed", i+1)
	}
}

func TestFixedWindow_BlockOverLimit(t *testing.T) {
	t.Parallel()

	// Arrange: 3 requests per second window.
	fw := ratelimit.NewFixedWindow(3, time.Second)

	const key = "fw-block"

	// Act: exhaust the 3-request quota.
	for range 3 {
		fw.Allow(key) //nolint:errcheck
	}
	blocked := fw.Allow(key)

	// Assert.
	assert.False(t, blocked, "4th request should be blocked within the same window")
}

func TestFixedWindow_WindowReset(t *testing.T) {
	// Arrange: 2 requests per 50ms window (small for test speed).
	fw := ratelimit.NewFixedWindow(2, 50*time.Millisecond)

	const key = "fw-reset"

	// Act: exhaust first window.
	require.True(t, fw.Allow(key), "1st request should be allowed")
	require.True(t, fw.Allow(key), "2nd request should be allowed")
	require.False(t, fw.Allow(key), "3rd request should be blocked")

	// Wait for window to expire.
	time.Sleep(60 * time.Millisecond)

	// Assert: new window allows requests again.
	assert.True(t, fw.Allow(key), "request after window reset should be allowed")
}
