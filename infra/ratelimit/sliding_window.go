// sliding_window.go — Sliding window log rate limiting with per-key timestamps.
package ratelimit

import (
	"sync"
	"time"
)

// SlidingWindow implements Strategy using the sliding window log algorithm.
// Each key maintains a log of request timestamps; only timestamps within
// the window duration are retained when evaluating a new request.
type SlidingWindow struct {
	mu         sync.Mutex
	timestamps map[string][]time.Time
	rate       int
	window     time.Duration
}

// NewSlidingWindow creates a SlidingWindow that allows rate requests per window duration.
func NewSlidingWindow(rate int, window time.Duration) *SlidingWindow {
	return &SlidingWindow{
		timestamps: make(map[string][]time.Time),
		rate:       rate,
		window:     window,
	}
}

// Allow reports whether a request from the given key should be allowed.
// Timestamps older than window are pruned before the check.
func (sw *SlidingWindow) Allow(key string) bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-sw.window)

	existing := sw.timestamps[key]
	pruned := pruneTimestamps(existing, cutoff)

	if len(pruned) >= sw.rate {
		sw.timestamps[key] = pruned
		return false
	}

	sw.timestamps[key] = append(pruned, now)
	return true
}

// pruneTimestamps returns only the timestamps that are after the cutoff time.
func pruneTimestamps(timestamps []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(timestamps) && !timestamps[i].After(cutoff) {
		i++
	}
	return timestamps[i:]
}
