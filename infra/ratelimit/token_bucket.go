// token_bucket.go — Token bucket rate limiting via x/time/rate with per-key limiters.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	cleanupInterval = 5 * time.Minute
	staleThreshold  = 10 * time.Minute
)

// tokenBucketEntry holds a per-key limiter and the last access time.
type tokenBucketEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// TokenBucket implements Strategy using the token bucket algorithm.
// Each key gets its own independent limiter. A background goroutine
// removes entries that have not been seen for 10 minutes.
type TokenBucket struct {
	mu      sync.RWMutex
	entries map[string]*tokenBucketEntry
	limit   rate.Limit
	burst   int
	stop    chan struct{}
}

// NewTokenBucket creates a TokenBucket that allows rps requests per second
// with a burst capacity of burst. Call Close() when done to release resources.
func NewTokenBucket(rps int, burst int) *TokenBucket {
	tb := &TokenBucket{
		entries: make(map[string]*tokenBucketEntry),
		limit:   rate.Limit(rps),
		burst:   burst,
		stop:    make(chan struct{}),
	}

	go tb.cleanupLoop()
	return tb
}

// Allow reports whether a request from the given key should be allowed.
func (tb *TokenBucket) Allow(key string) bool {
	return tb.getLimiter(key).Allow()
}

// Close stops the background cleanup goroutine. Safe to call once.
func (tb *TokenBucket) Close() {
	close(tb.stop)
}

func (tb *TokenBucket) getLimiter(key string) *rate.Limiter {
	now := time.Now()

	tb.mu.RLock()
	entry, exists := tb.entries[key]
	tb.mu.RUnlock()

	if exists {
		tb.mu.Lock()
		entry.lastSeen = now
		tb.mu.Unlock()
		return entry.limiter
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Double-check after acquiring write lock.
	if entry, exists = tb.entries[key]; exists {
		entry.lastSeen = now
		return entry.limiter
	}

	limiter := rate.NewLimiter(tb.limit, tb.burst)
	tb.entries[key] = &tokenBucketEntry{limiter: limiter, lastSeen: now}
	return limiter
}

func (tb *TokenBucket) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tb.removeStale()
		case <-tb.stop:
			return
		}
	}
}

func (tb *TokenBucket) removeStale() {
	cutoff := time.Now().Add(-staleThreshold)

	tb.mu.Lock()
	defer tb.mu.Unlock()

	for key, entry := range tb.entries {
		if entry.lastSeen.Before(cutoff) {
			delete(tb.entries, key)
		}
	}
}
