// Package http provides the production HTTP server for the wolf-be service.
package http

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// checkFn is the function signature for a single readiness probe.
type checkFn func(ctx context.Context) error

// checkResult holds the outcome of a single named readiness check.
type checkResult struct {
	// Status is "ok" when the check passed, "error" otherwise.
	Status string `json:"status"`
	// Error contains the error message when Status is "error".
	Error string `json:"error,omitempty"`
}

// readinessResponse is the JSON body returned by the readiness handler.
type readinessResponse struct {
	// Status is "ok" when all checks passed, "error" otherwise.
	Status string `json:"status"`
	// Checks is the per-check breakdown.
	Checks map[string]checkResult `json:"checks"`
}

// ReadinessChecker runs a named set of dependency health checks.
// Checks are added via Add and executed concurrently by Handler.
type ReadinessChecker struct {
	mu     sync.RWMutex
	checks map[string]checkFn
}

// NewReadinessChecker returns an empty *ReadinessChecker ready to accept
// named check functions.
func NewReadinessChecker() *ReadinessChecker {
	return &ReadinessChecker{
		checks: make(map[string]checkFn),
	}
}

// Add registers a named readiness check. name should be a short, descriptive
// label (e.g. "postgres", "redis"). Adding a check with a duplicate name
// replaces the existing one.
func (r *ReadinessChecker) Add(name string, check func(ctx context.Context) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks[name] = check
}

// Handler returns a gin.HandlerFunc that runs all registered checks with a
// three-second timeout and responds with HTTP 200 when all pass or HTTP 503
// when one or more fail. The response body is a JSON object describing each
// check's outcome.
func (r *ReadinessChecker) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		r.mu.RLock()
		// Snapshot the map so we can release the lock before running I/O.
		snapshot := make(map[string]checkFn, len(r.checks))
		for name, fn := range r.checks {
			snapshot[name] = fn
		}
		r.mu.RUnlock()

		type namedResult struct {
			name   string
			result checkResult
		}

		resultCh := make(chan namedResult, len(snapshot))

		for name, fn := range snapshot {
			name, fn := name, fn
			go func() {
				cr := checkResult{Status: "ok"}
				if err := fn(ctx); err != nil {
					cr.Status = "error"
					cr.Error = err.Error()
				}
				resultCh <- namedResult{name: name, result: cr}
			}()
		}

		results := make(map[string]checkResult, len(snapshot))
		overallOK := true

		for range snapshot {
			nr := <-resultCh
			results[nr.name] = nr.result
			if nr.result.Status != "ok" {
				overallOK = false
			}
		}

		resp := readinessResponse{
			Status: "ok",
			Checks: results,
		}
		statusCode := http.StatusOK

		if !overallOK {
			resp.Status = "error"
			statusCode = http.StatusServiceUnavailable
		}

		JSON(c, statusCode, resp)
	}
}
