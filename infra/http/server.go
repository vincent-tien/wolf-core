// Package http provides the production HTTP server for the wolf-be service.
// It wraps Gin with standardised middleware, system endpoints, and a
// net/http.Server configured from HTTPConfig.
package http

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof" // register pprof handlers on the default mux

	"github.com/felixge/fgprof"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/config"
	"github.com/vincent-tien/wolf-core/infra/observability/metrics"
)

// Server is the production HTTP server. It owns a Gin engine, a net/http.Server,
// and exposes the versioned API router for modules to register their routes.
type Server struct {
	engine     *gin.Engine
	httpServer *http.Server
	logger     *zap.Logger
	metrics    *metrics.Metrics
	apiV1      gin.IRouter
	readiness  *ReadinessChecker
}

// New creates a fully configured *Server from the provided configuration.
// When appCfg.Debug is false, Gin is put in release mode before the engine is
// built. The following system endpoints are always registered:
//
//   - GET /health  — liveness probe, always returns HTTP 200.
//   - GET /ready   — readiness probe, delegated to ReadinessChecker.
//
// Prometheus metrics are served on a dedicated MetricsServer (separate port)
// to keep scrape traffic off the API and bypass auth/rate-limit middleware.
//
// When appCfg.Debug is true, pprof endpoints are also mounted under
// /debug/pprof/.
func New(cfg config.HTTPConfig, appCfg config.AppConfig, logger *zap.Logger, m *metrics.Metrics) *Server {
	if !appCfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()

	// Configure trusted proxies so that c.ClientIP() returns the real client
	// IP instead of the load balancer address. Critical for per-IP rate
	// limiting, audit logging, and abuse detection.
	if len(cfg.TrustedProxies) > 0 {
		if err := engine.SetTrustedProxies(cfg.TrustedProxies); err != nil {
			logger.Error("failed to set trusted proxies", zap.Error(err))
		}
	} else {
		// No proxies configured — trust nothing. Gin defaults to trusting all,
		// which is unsafe behind any reverse proxy.
		_ = engine.SetTrustedProxies(nil)
	}

	// NOTE: Do NOT add gin.Recovery() here. The custom Recovery(logger) in
	// BuildChain handles panics with structured JSON responses and logging.

	readiness := NewReadinessChecker()

	// System endpoints — always available.
	engine.GET("/health", func(c *gin.Context) {
		JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})
	engine.GET("/ready", readiness.Handler())

	// pprof endpoints — debug mode only and never in production.
	if appCfg.Debug && appCfg.Env != config.EnvProduction {
		// The pprof init() registered all handlers on http.DefaultServeMux;
		// we proxy the prefix to that mux from Gin.
		engine.GET("/debug/pprof/*any", gin.WrapH(http.DefaultServeMux))

		// fgprof captures both on-CPU and off-CPU (blocked) goroutine time,
		// complementing the standard CPU profiler.
		engine.GET("/debug/fgprof/profile", gin.WrapH(fgprof.Handler()))
	}

	apiV1 := engine.Group("/api/v1")

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           engine,
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    cfg.MaxHeaderBytes,
	}

	return &Server{
		engine:     engine,
		httpServer: httpServer,
		logger:     logger,
		metrics:    m,
		apiV1:      apiV1,
		readiness:  readiness,
	}
}

// Router returns the /api/v1 router group. Application modules call this to
// register their own route sets.
func (s *Server) Router() gin.IRouter {
	return s.apiV1
}

// Engine returns the underlying *gin.Engine, giving callers full access for
// advanced customisation (e.g. adding middleware after construction).
func (s *Server) Engine() *gin.Engine {
	return s.engine
}

// Readiness returns the *ReadinessChecker so that infrastructure components
// (database, cache, broker) can register dependency health checks.
func (s *Server) Readiness() *ReadinessChecker {
	return s.readiness
}

// Start begins accepting connections on the configured port. It blocks until
// the server is shut down or encounters a fatal error. Callers should run
// Start in a separate goroutine and use Stop to initiate a graceful shutdown.
//
// ErrServerClosed from net/http is not treated as an error; it is returned as
// nil to signal a clean shutdown.
func (s *Server) Start() error {
	s.logger.Info("HTTP server starting", zap.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// Stop performs a graceful shutdown of the HTTP server. It waits for active
// connections to finish up to the deadline carried by ctx.
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("HTTP server stopping")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("http server shutdown: %w", err)
	}
	return nil
}
