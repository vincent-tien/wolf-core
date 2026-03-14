package ratelimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/ratelimit"
)

func TestSlidingWindow_AllowUnderLimit(t *testing.T) {
	t.Parallel()

	// Arrange: 5 requests per second.
	sw := ratelimit.NewSlidingWindow(5, time.Second)

	const key = "sw-user-1"

	// Act & Assert: all 5 requests within the window must be allowed.
	for i := range 5 {
		got := sw.Allow(key)
		require.True(t, got, "expected request %d to be allowed", i+1)
	}
}

func TestSlidingWindow_BlockOverLimit(t *testing.T) {
	t.Parallel()

	// Arrange: 3 requests per second.
	sw := ratelimit.NewSlidingWindow(3, time.Second)

	const key = "sw-block"

	// Act: exhaust the 3-request quota.
	for range 3 {
		sw.Allow(key) //nolint:errcheck
	}
	blocked := sw.Allow(key)

	// Assert.
	assert.False(t, blocked, "4th request should be blocked within the window")
}

func TestSlidingWindow_SlidesCorrectly(t *testing.T) {
	// Arrange: 2 requests per 80ms window.
	// Use small window to keep test fast.
	sw := ratelimit.NewSlidingWindow(2, 80*time.Millisecond)

	const key = "sw-slide"

	// Act: fill the window.
	require.True(t, sw.Allow(key), "1st request should be allowed")
	require.True(t, sw.Allow(key), "2nd request should be allowed")
	require.False(t, sw.Allow(key), "3rd request should be blocked")

	// Wait for both timestamps to slide out of the window.
	time.Sleep(90 * time.Millisecond)

	// Assert: the window has slid forward; old timestamps are pruned.
	assert.True(t, sw.Allow(key), "request after window slide should be allowed")
	assert.True(t, sw.Allow(key), "second request after window slide should be allowed")
	assert.False(t, sw.Allow(key), "third request after window slide should still be blocked")
}
