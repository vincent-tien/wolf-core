// metrics.go — Messenger middleware that records Prometheus dispatch metrics.
package middleware

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vincent-tien/wolf-core/messenger"
)

// Metrics records dispatch counts and durations via Prometheus.
type Metrics struct {
	dispatches *prometheus.CounterVec
	duration   *prometheus.HistogramVec
	busName    string
}

// NewMetrics creates a metrics middleware.
// Registers Prometheus metrics at creation time (not per-request).
func NewMetrics(busName string, reg prometheus.Registerer) *Metrics {
	dispatches := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "messenger_dispatches_total",
		Help: "Total number of message dispatches",
	}, []string{"type", "bus", "status"})

	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "messenger_dispatch_duration_seconds",
		Help:    "Duration of message dispatch",
		Buckets: prometheus.DefBuckets,
	}, []string{"type", "bus"})

	reg.MustRegister(dispatches, duration)

	return &Metrics{
		dispatches: dispatches,
		duration:   duration,
		busName:    busName,
	}
}

// NewMetricsFrom creates a metrics middleware from pre-registered metric vectors.
func NewMetricsFrom(busName string, dispatches *prometheus.CounterVec, duration *prometheus.HistogramVec) *Metrics {
	return &Metrics{
		dispatches: dispatches,
		duration:   duration,
		busName:    busName,
	}
}

func (m *Metrics) Handle(ctx context.Context, env messenger.Envelope, next messenger.MiddlewareNext) (messenger.DispatchResult, error) {
	msgType := env.MessageTypeName()
	start := time.Now()

	result, err := next(ctx, env)
	elapsed := time.Since(start).Seconds()

	m.duration.WithLabelValues(msgType, m.busName).Observe(elapsed)

	status := "success"
	if err != nil {
		status = "error"
	} else if result.Async {
		status = "async_sent"
	}
	m.dispatches.WithLabelValues(msgType, m.busName, status).Inc()

	return result, err
}
