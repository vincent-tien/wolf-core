// strategy.go — Rate limiting Strategy interface and Config struct.
package ratelimit

import "time"

// Strategy is the interface for rate limiting algorithms.
type Strategy interface {
	// Allow reports whether a request from the given key should be allowed.
	Allow(key string) bool
}

// Config holds the configuration for creating a rate limiter.
type Config struct {
	// Algorithm is the rate limiting algorithm: "token_bucket", "fixed_window", or "sliding_window".
	Algorithm string
	// Rate is the number of requests allowed per window.
	Rate int
	// Burst is the burst capacity (only used by token_bucket).
	Burst int
	// Window is the time window size (used by fixed_window and sliding_window).
	Window time.Duration
}
