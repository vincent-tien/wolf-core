// factory.go — Creates rate limiter Strategy from config (token_bucket/fixed_window/sliding_window).
package ratelimit

import "fmt"

const (
	AlgorithmTokenBucket    = "token_bucket"
	AlgorithmFixedWindow    = "fixed_window"
	AlgorithmSlidingWindow  = "sliding_window"
)

// New creates a Strategy from the given Config.
// Returns an error for unknown algorithms.
func New(cfg Config) (Strategy, error) {
	switch cfg.Algorithm {
	case AlgorithmTokenBucket:
		return NewTokenBucket(cfg.Rate, cfg.Burst), nil
	case AlgorithmFixedWindow:
		return NewFixedWindow(cfg.Rate, cfg.Window), nil
	case AlgorithmSlidingWindow:
		return NewSlidingWindow(cfg.Rate, cfg.Window), nil
	default:
		return nil, fmt.Errorf("ratelimit: unknown algorithm %q", cfg.Algorithm)
	}
}
