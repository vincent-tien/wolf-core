// rate_limit.go — Per-IP rate limiting middleware with token bucket.
package http

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// PerIPRateLimiter enforces per-client-IP rate limiting using a map of
// token-bucket limiters. Stale entries are cleaned up periodically.
// MaxClients caps the map size to prevent memory exhaustion under DDoS.
type PerIPRateLimiter struct {
	mu         sync.RWMutex
	clients    map[string]*clientEntry
	limit      rate.Limit
	burst      int
	maxClients int
	stop       chan struct{}
}

type clientEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// defaultMaxClients caps the per-IP map to prevent memory exhaustion.
const defaultMaxClients = 100_000

// NewPerIPRateLimiter creates a per-IP rate limiter.
// rps = requests per second per client, burst = max burst size.
func NewPerIPRateLimiter(rps int, burst int) *PerIPRateLimiter {
	rl := &PerIPRateLimiter{
		clients:    make(map[string]*clientEntry),
		limit:      rate.Limit(rps),
		burst:      burst,
		maxClients: defaultMaxClients,
		stop:       make(chan struct{}),
	}

	go rl.cleanupLoop()
	return rl
}

// Close stops the background cleanup goroutine. Call during graceful shutdown.
func (rl *PerIPRateLimiter) Close() {
	close(rl.stop)
}

func (rl *PerIPRateLimiter) getLimiter(clientIP string) *rate.Limiter {
	now := time.Now()

	rl.mu.RLock()
	entry, exists := rl.clients[clientIP]
	rl.mu.RUnlock()

	if exists {
		rl.mu.Lock()
		entry.lastSeen = now
		rl.mu.Unlock()
		return entry.limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after write lock.
	if entry, exists = rl.clients[clientIP]; exists {
		entry.lastSeen = now
		return entry.limiter
	}

	// Evict a random entry if at capacity to prevent unbounded memory growth.
	if len(rl.clients) >= rl.maxClients {
		rl.evictOne()
	}

	limiter := rate.NewLimiter(rl.limit, rl.burst)
	rl.clients[clientIP] = &clientEntry{limiter: limiter, lastSeen: now}
	return limiter
}

// evictOne removes a single entry from the map to make room. Go's map
// iteration order is randomised, so the first entry is effectively a random
// eviction — O(1) instead of the previous O(n) scan for the oldest entry.
// Precise LRU eviction is unnecessary here because the background cleanup
// goroutine already removes entries not seen for 3 minutes on a 1-minute
// tick, handling steady-state staleness.
func (rl *PerIPRateLimiter) evictOne() {
	for ip := range rl.clients {
		delete(rl.clients, ip)
		return
	}
}

// Middleware returns a Gin middleware that rate limits by client IP.
func (rl *PerIPRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		limiter := rl.getLimiter(clientIP)

		if !limiter.Allow() {
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			return
		}

		c.Next()
	}
}

// cleanupLoop removes client entries that have not been seen for 3 minutes.
// Runs every 1 minute to limit memory growth under high-cardinality traffic.
func (rl *PerIPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-3 * time.Minute)
			for ip, entry := range rl.clients {
				if entry.lastSeen.Before(cutoff) {
					delete(rl.clients, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}
