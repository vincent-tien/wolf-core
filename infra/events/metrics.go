// metrics.go — Prometheus instrumentation for message publish and receive operations.
package events

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/vincent-tien/wolf-core/messaging"
)

// MessageMetrics holds Prometheus counters for message publish and receive operations.
type MessageMetrics struct {
	SentTotal     *prometheus.CounterVec
	ReceivedTotal *prometheus.CounterVec
}

// NewMessageMetrics creates and registers Prometheus counters for messaging.
//
// The registerer parameter allows using a custom registry (e.g., for testing)
// instead of the global default. Labels are "subject" and "status", where
// status is either "ok" or "error".
//
// Example composition with tracing:
//
//	pub = NewTracingPublisher(pub)
//	pub = NewMetricsPublisher(pub, metrics)
func NewMessageMetrics(registerer prometheus.Registerer) *MessageMetrics {
	sent := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "wolf",
		Subsystem: "messaging",
		Name:      "sent_total",
		Help:      "Total number of messages published, partitioned by subject and status.",
	}, []string{"subject", "status"})

	received := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "wolf",
		Subsystem: "messaging",
		Name:      "received_total",
		Help:      "Total number of messages received, partitioned by subject and status.",
	}, []string{"subject", "status"})

	registerer.MustRegister(sent, received)

	return &MessageMetrics{
		SentTotal:     sent,
		ReceivedTotal: received,
	}
}

// MetricsPublisher wraps a messaging.Publisher and increments sent counters
// on every Publish call, recording the subject and outcome (ok/error).
type MetricsPublisher struct {
	next    messaging.Publisher
	metrics *MessageMetrics
}

// NewMetricsPublisher wraps next with Prometheus publish instrumentation.
func NewMetricsPublisher(next messaging.Publisher, metrics *MessageMetrics) *MetricsPublisher {
	return &MetricsPublisher{next: next, metrics: metrics}
}

// Publish delegates to the wrapped publisher and records the outcome in the
// SentTotal counter with labels for subject and status ("ok" or "error").
func (p *MetricsPublisher) Publish(ctx context.Context, subject string, msg messaging.RawMessage) error {
	err := p.next.Publish(ctx, subject, msg)
	status := "ok"
	if err != nil {
		status = "error"
	}
	p.metrics.SentTotal.WithLabelValues(subject, status).Inc()
	return err
}

// MetricsSubscribeMiddleware returns a MessageHandler middleware that increments
// the ReceivedTotal counter for each message processed, labeled by subject and
// status ("ok" or "error").
func MetricsSubscribeMiddleware(metrics *MessageMetrics) func(messaging.MessageHandler) messaging.MessageHandler {
	return func(next messaging.MessageHandler) messaging.MessageHandler {
		return func(ctx context.Context, msg messaging.Message) error {
			err := next(ctx, msg)
			status := "ok"
			if err != nil {
				status = "error"
			}
			metrics.ReceivedTotal.WithLabelValues(msg.Subject(), status).Inc()
			return err
		}
	}
}
