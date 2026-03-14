// http_client.go — Production-grade HTTP client with retry + circuit breaker.
package resilience

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

// ResilientHTTPClient wraps http.Client with timeout, retry, and circuit
// breaker. The composition order is:
//
//	caller → retry → circuit breaker → http.Client (with timeout)
//
// This ensures transient failures are retried, while sustained failures trip
// the circuit breaker and fail fast.
type ResilientHTTPClient struct {
	client     *http.Client
	cb         *gobreaker.CircuitBreaker
	maxRetries int
	baseDelay  time.Duration
}

// NewResilientHTTPClient creates an HTTP client that combines:
//   - Per-request timeout
//   - Exponential backoff retry (±25% jitter)
//   - Circuit breaker (trips on >50% error rate after 5+ requests)
func NewResilientHTTPClient(name string, timeout time.Duration, maxRetries int, logger *zap.Logger) *ResilientHTTPClient {
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name,
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Require at least 5 requests before evaluating the failure ratio
			// to avoid tripping on transient single-request failures.
			if counts.Requests < 5 {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio > 0.5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Info("http client circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
		},
	})

	return &ResilientHTTPClient{
		client:     &http.Client{Timeout: timeout},
		cb:         cb,
		maxRetries: maxRetries,
		baseDelay:  100 * time.Millisecond,
	}
}

// Do executes an HTTP request through the resilience stack.
// It returns the response only on success (2xx-4xx from the upstream).
// Server errors (5xx) are treated as circuit breaker failures and retried.
func (c *ResilientHTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	var resp *http.Response

	err := WithRetry(ctx, c.maxRetries, c.baseDelay, func() error {
		result, cbErr := c.cb.Execute(func() (any, error) {
			req = req.WithContext(ctx)
			r, err := c.client.Do(req)
			if err != nil {
				return nil, err
			}
			if r.StatusCode >= http.StatusInternalServerError {
				r.Body.Close()
				return nil, fmt.Errorf("resilience: server error %d", r.StatusCode)
			}
			return r, nil
		})

		if cbErr != nil {
			return cbErr
		}
		resp = result.(*http.Response)
		return nil
	})

	return resp, err
}
