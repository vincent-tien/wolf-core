// load_shedder.go — Adaptive load shedding that returns 503 when in-flight requests exceed threshold.
package http

import (
	"sync/atomic"

	"github.com/gin-gonic/gin"

	"github.com/vincent-tien/wolf-core/infra/config"
	wolfhttp "github.com/vincent-tien/wolf-core/infra/http"
)

// LoadShedder tracks in-flight requests and rejects new ones when the
// concurrency limit is reached. Unlike rate limiting (which caps throughput),
// load shedding caps parallelism — preventing goroutine/memory pile-up under
// traffic spikes that would otherwise cause cascading timeouts or OOM.
//
// Design decisions:
//   - atomic.Int64 (not sync.Mutex) — lock-free, zero contention on the hot path.
//   - Decrement via defer — guarantees the counter decreases even on panic,
//     preventing a leaked count that would permanently reject requests.
//   - Retry-After header — signals clients to back off, preventing retry storms.
//
// Sizing: set limit to CPU cores × 100–200 for I/O-heavy APIs. Benchmark under
// realistic load to find the sweet spot before cascading timeouts begin.
type LoadShedder struct {
	inflight atomic.Int64
	limit    int64
}

// NewLoadShedder creates a LoadShedder that rejects requests when the number
// of concurrently executing handlers exceeds limit.
func NewLoadShedder(limit int) *LoadShedder {
	return &LoadShedder{limit: int64(limit)}
}

// Middleware returns a Gin handler that responds 503 Service Unavailable when
// the in-flight request count exceeds the configured limit.
func (ls *LoadShedder) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		current := ls.inflight.Add(1)
		defer ls.inflight.Add(-1)

		if current > ls.limit {
			wolfhttp.AbortServiceUnavailable(c, "server at capacity")
			return
		}

		c.Next()
	}
}

// InFlight returns the current number of in-flight requests. Useful for
// Prometheus gauge instrumentation.
func (ls *LoadShedder) InFlight() int64 {
	return ls.inflight.Load()
}

// LoadShed returns a Gin middleware that limits concurrent in-flight requests.
// When cfg.MaxConcurrent is 0, the middleware is a no-op passthrough.
func LoadShed(cfg config.LoadShedConfig) gin.HandlerFunc {
	if cfg.MaxConcurrent <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	shedder := NewLoadShedder(cfg.MaxConcurrent)
	return shedder.Middleware()
}
