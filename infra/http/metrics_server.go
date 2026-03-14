// metrics_server.go — Dedicated HTTP server for Prometheus /metrics endpoint.
package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// MetricsServer serves Prometheus metrics on a dedicated port, separate from
// the main API server. This keeps scrape traffic off the API port and matches
// the Kubernetes deployment annotations (prometheus.io/port: "9091").
//
// Running metrics on a separate port also means Prometheus scrapes bypass API
// middleware (auth, rate-limit, load-shed) and remain available even when the
// main server is overloaded.
type MetricsServer struct {
	server *http.Server
	logger *zap.Logger
}

// NewMetricsServer creates a lightweight HTTP server that serves /metrics on
// the given port.
func NewMetricsServer(port int, logger *zap.Logger) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	return &MetricsServer{
		server: &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  30 * time.Second,
		},
		logger: logger,
	}
}

// Start begins serving metrics. Blocks until stopped or a fatal error occurs.
func (s *MetricsServer) Start() error {
	s.logger.Info("metrics server starting", zap.String("addr", s.server.Addr))
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics server: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the metrics server.
func (s *MetricsServer) Stop(ctx context.Context) error {
	s.logger.Info("metrics server stopping")
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("metrics server shutdown: %w", err)
	}
	return nil
}
