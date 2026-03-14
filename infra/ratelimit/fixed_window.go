// fixed_window.go — Fixed-window rate limiting with per-key counters.
package ratelimit

import (
	"sync"
	"time"
)

// fixedWindowEntry holds the counter and window start time for a single key.
type fixedWindowEntry struct {
	count       int
	windowStart time.Time
}

// FixedWindow implements Strategy using the fixed window counter algorithm.
// Each key gets its own counter that resets at fixed time intervals.
type FixedWindow struct {
	mu      sync.Mutex
	entries map[string]*fixedWindowEntry
	rate    int
	window  time.Duration
}

// NewFixedWindow creates a FixedWindow that allows rate requests per window duration.
func NewFixedWindow(rate int, window time.Duration) *FixedWindow {
	return &FixedWindow{
		entries: make(map[string]*fixedWindowEntry),
		rate:    rate,
		window:  window,
	}
}

// Allow reports whether a request from the given key should be allowed.
// The counter resets when the current window expires.
func (fw *FixedWindow) Allow(key string) bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	now := time.Now()
	entry, exists := fw.entries[key]

	if !exists {
		fw.entries[key] = &fixedWindowEntry{count: 1, windowStart: now}
		return true
	}

	if now.After(entry.windowStart.Add(fw.window)) {
		entry.windowStart = now
		entry.count = 1
		return true
	}

	if entry.count >= fw.rate {
		return false
	}

	entry.count++
	return true
}
