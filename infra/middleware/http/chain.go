// chain.go — Ordered middleware chain builder for the Gin HTTP server.
package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/config"
	"github.com/vincent-tien/wolf-core/infra/observability/metrics"
)

// ChainDeps holds the shared dependencies required to build the default
// middleware chain.
type ChainDeps struct {
	// Logger is the application-wide structured logger.
	Logger *zap.Logger
	// Metrics holds the Prometheus collectors for HTTP request instrumentation.
	Metrics *metrics.Metrics
	// PerIPLimiter is the per-client-IP rate limiter. When non-nil its middleware
	// is appended to the chain. The caller owns the lifecycle (Close on shutdown).
	PerIPLimiter *PerIPRateLimiter
	// LoadShed holds concurrency-based load shedding configuration.
	LoadShed config.LoadShedConfig
	// Timeout is the maximum duration allowed for a single HTTP request.
	Timeout time.Duration
	// ServiceName is the logical service name embedded in tracing spans.
	ServiceName string
	// SecurityHeaders configures HTTP security response headers.
	// When Enabled is true the middleware is inserted as the second handler,
	// immediately after Recovery so that security headers are present on all
	// responses including error and panic recovery responses.
	SecurityHeaders config.SecurityHeadersConfig
	// CORS holds cross-origin resource sharing configuration from config.yaml.
	// When empty, the CORS middleware falls back to permissive defaults
	// (AllowedOrigins: ["*"]). Production deployments MUST set explicit origins.
	CORS config.CORSConfig
}

// BuildChain returns the default ordered middleware chain for HTTP handlers.
// The chain is designed to be registered once on the root router group via
// router.Use(BuildChain(deps)...).
//
// Order:
//  1. Recovery        – catch panics from all downstream handlers.
//  2. SecurityHeaders – set HTTP security headers on every response (when enabled).
//  3. RequestID       – assign a unique ID before anything logs.
//  4. Tracing         – create root span (must precede logger for trace_id).
//  5. Logging         – structured log with trace_id, request_id, duration.
//  6. Metrics         – record request count, latency, in-flight gauge.
//  7. LoadShed        – reject when in-flight requests exceed capacity (503).
//  8. Timeout         – enforce maximum request duration.
//  9. CORS            – handle preflight requests.
//  10. RateLimit      – throttle abusive clients.
//
// Auth, RBAC, and Transaction middleware remain per-route-group (not global).
func BuildChain(deps ChainDeps) []gin.HandlerFunc {
	chain := []gin.HandlerFunc{
		Recovery(deps.Logger),
	}

	if deps.SecurityHeaders.Enabled {
		chain = append(chain, SecurityHeaders(deps.SecurityHeaders))
	}

	chain = append(chain,
		RequestID(),
		Tracing(deps.ServiceName),
		Logging(deps.Logger),
		Metrics(deps.Metrics),
	)

	if deps.LoadShed.MaxConcurrent > 0 {
		chain = append(chain, LoadShed(deps.LoadShed))
	}

	if deps.Timeout > 0 {
		chain = append(chain, Timeout(deps.Timeout))
	}

	chain = append(chain, CORS(CORSConfig{
		AllowedOrigins:   deps.CORS.AllowedOrigins,
		AllowedMethods:   deps.CORS.AllowedMethods,
		AllowedHeaders:   deps.CORS.AllowedHeaders,
		AllowCredentials: deps.CORS.AllowCredentials,
		MaxAge:           deps.CORS.MaxAge,
	}))

	if deps.PerIPLimiter != nil {
		chain = append(chain, deps.PerIPLimiter.Middleware())
	}

	return chain
}
