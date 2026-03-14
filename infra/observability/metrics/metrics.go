// Package metrics provides Prometheus metric collectors for the wolf-be service.
// All collectors are registered against the default Prometheus registry on
// construction via New(). Callers should create a single *Metrics instance and
// share it across the application (e.g. via dependency injection).
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// defaultBuckets are the histogram observation buckets shared across all
// latency histograms in the service.
var defaultBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// Metrics holds all Prometheus collectors used by the service.
// Fields are grouped by concern: HTTP, gRPC, database, cache, outbox, events.
type Metrics struct {
	// -------------------------------------------------------------------
	// HTTP
	// -------------------------------------------------------------------

	// HTTPRequestDuration tracks the latency of HTTP requests, labelled by
	// method, path, and status code.
	HTTPRequestDuration *prometheus.HistogramVec

	// HTTPRequestTotal counts the number of HTTP requests completed, labelled
	// by method, path, and status code.
	HTTPRequestTotal *prometheus.CounterVec

	// HTTPRequestsInFlight tracks the number of HTTP requests currently being
	// served.
	HTTPRequestsInFlight prometheus.Gauge

	// -------------------------------------------------------------------
	// gRPC
	// -------------------------------------------------------------------

	// GRPCRequestDuration tracks the latency of gRPC calls, labelled by
	// method and status.
	GRPCRequestDuration *prometheus.HistogramVec

	// GRPCRequestTotal counts the number of gRPC calls completed, labelled by
	// method and status.
	GRPCRequestTotal *prometheus.CounterVec

	// -------------------------------------------------------------------
	// Database
	// -------------------------------------------------------------------

	// DBQueryDuration tracks the latency of database queries, labelled by
	// pool name and query name.
	DBQueryDuration *prometheus.HistogramVec

	// -------------------------------------------------------------------
	// Outbox
	// -------------------------------------------------------------------

	// OutboxQueueDepth tracks the number of pending messages in the
	// transactional outbox table.
	OutboxQueueDepth prometheus.Gauge

	// OutboxPublishTotal counts the number of outbox publish attempts,
	// labelled by outcome status ("success" or "error").
	OutboxPublishTotal *prometheus.CounterVec

	// OutboxPolledTotal counts the number of entries fetched from the outbox
	// per poll cycle.
	OutboxPolledTotal prometheus.Counter

	// OutboxDLQTotal counts the number of entries moved to the dead letter
	// queue after exhausting retries.
	OutboxDLQTotal prometheus.Counter

	// OutboxDLQInsertErrors counts the number of times DLQ insert failed.
	// Non-zero means events are stuck in the outbox and cannot be moved to
	// DLQ — requires operator attention.
	OutboxDLQInsertErrors prometheus.Counter

	// OutboxMarkPublishedErrors counts the number of times MarkPublished
	// failed after successful broker delivery. This means events were
	// delivered but the outbox row wasn't updated — causing re-delivery.
	OutboxMarkPublishedErrors prometheus.Counter

	// OutboxPollDuration tracks the latency of each outbox poll cycle.
	OutboxPollDuration prometheus.Histogram

	// -------------------------------------------------------------------
	// Events
	// -------------------------------------------------------------------

	// EventPublishTotal counts the number of domain events published,
	// labelled by event_type and status.
	EventPublishTotal *prometheus.CounterVec

	// EventConsumeTotal counts the number of domain events consumed,
	// labelled by event_type and status.
	EventConsumeTotal *prometheus.CounterVec

	// EventConsumeDuration tracks the processing latency per consumed event,
	// labelled by event_type.
	EventConsumeDuration *prometheus.HistogramVec

	// -------------------------------------------------------------------
	// CQRS Cache (per-query cache hit/miss visibility)
	// -------------------------------------------------------------------

	// QueryCacheHits counts cache hits in CQRS query caching middleware,
	// labelled by query_name.
	QueryCacheHits *prometheus.CounterVec

	// QueryCacheMisses counts cache misses in CQRS query caching middleware,
	// labelled by query_name.
	QueryCacheMisses *prometheus.CounterVec

	// -------------------------------------------------------------------
	// CQRS
	// -------------------------------------------------------------------

	// CommandDuration tracks the latency of command handler execution,
	// labelled by command_name and status.
	CommandDuration *prometheus.HistogramVec

	// CommandTotal counts the number of command handler executions,
	// labelled by command_name and status.
	CommandTotal *prometheus.CounterVec

	// QueryDuration tracks the latency of query handler execution,
	// labelled by query_name and status.
	QueryDuration *prometheus.HistogramVec

	// QueryTotal counts the number of query handler executions,
	// labelled by query_name and status.
	QueryTotal *prometheus.CounterVec
}

// singleton holds the package-level Metrics instance to ensure collectors
// are registered exactly once with the default Prometheus registry.
var (
	singleton     *Metrics
	singletonOnce sync.Once
)

// New creates, registers, and returns a fully initialised *Metrics instance.
// Repeated calls return the same instance — Prometheus panics on duplicate
// registration, so sync.Once guarantees collectors are registered exactly once.
func New() *Metrics {
	singletonOnce.Do(func() {
		singleton = newMetrics()
	})
	return singleton
}

// newMetrics performs the actual collector construction and registration.
func newMetrics() *Metrics {
	m := &Metrics{
		// -------------------------------------------------------------------
		// HTTP
		// -------------------------------------------------------------------
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds.",
				Buckets: defaultBuckets,
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests completed.",
			},
			[]string{"method", "path", "status"},
		),
		HTTPRequestsInFlight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "http_requests_in_flight",
				Help: "Number of HTTP requests currently being served.",
			},
		),

		// -------------------------------------------------------------------
		// gRPC
		// -------------------------------------------------------------------
		GRPCRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "grpc_request_duration_seconds",
				Help:    "Duration of gRPC calls in seconds.",
				Buckets: defaultBuckets,
			},
			[]string{"method", "status"},
		),
		GRPCRequestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "grpc_requests_total",
				Help: "Total number of gRPC calls completed.",
			},
			[]string{"method", "status"},
		),

		// -------------------------------------------------------------------
		// Database
		// -------------------------------------------------------------------
		DBQueryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "db_query_duration_seconds",
				Help:    "Duration of database queries in seconds.",
				Buckets: defaultBuckets,
			},
			[]string{"pool", "query"},
		),

		// -------------------------------------------------------------------
		// Outbox
		// -------------------------------------------------------------------
		OutboxQueueDepth: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "outbox_queue_depth",
				Help: "Number of pending messages in the transactional outbox.",
			},
		),
		OutboxPublishTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "outbox_publish_total",
				Help: "Total number of outbox publish attempts.",
			},
			[]string{"status"},
		),
		OutboxPolledTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "outbox_polled_total",
				Help: "Total number of entries fetched from the outbox.",
			},
		),
		OutboxDLQTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "outbox_dlq_total",
				Help: "Total number of entries moved to the dead letter queue.",
			},
		),
		OutboxDLQInsertErrors: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "outbox_dlq_insert_errors_total",
				Help: "Number of DLQ insert failures — events stuck in outbox, requires operator attention.",
			},
		),
		OutboxMarkPublishedErrors: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "outbox_mark_published_errors_total",
				Help: "Number of MarkPublished failures after successful broker delivery.",
			},
		),
		OutboxPollDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "outbox_poll_duration_seconds",
				Help:    "Duration of each outbox poll cycle in seconds.",
				Buckets: defaultBuckets,
			},
		),

		// -------------------------------------------------------------------
		// Events
		// -------------------------------------------------------------------
		EventPublishTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "event_publish_total",
				Help: "Total number of domain events published.",
			},
			[]string{"event_type", "status"},
		),
		EventConsumeTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "event_consume_total",
				Help: "Total number of domain events consumed.",
			},
			[]string{"event_type", "status"},
		),
		EventConsumeDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "event_consume_duration_seconds",
				Help:    "Processing latency of consumed domain events in seconds.",
				Buckets: defaultBuckets,
			},
			[]string{"event_type"},
		),

		// -------------------------------------------------------------------
		// CQRS Cache
		// -------------------------------------------------------------------
		QueryCacheHits: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cqrs_query_cache_hits_total",
				Help: "Total cache hits in CQRS query caching middleware.",
			},
			[]string{"query_name"},
		),
		QueryCacheMisses: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cqrs_query_cache_misses_total",
				Help: "Total cache misses in CQRS query caching middleware.",
			},
			[]string{"query_name"},
		),

		// -------------------------------------------------------------------
		// CQRS
		// -------------------------------------------------------------------
		CommandDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cqrs_command_duration_seconds",
				Help:    "Duration of command handler execution in seconds.",
				Buckets: defaultBuckets,
			},
			[]string{"command_name", "status"},
		),
		CommandTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cqrs_command_total",
				Help: "Total number of command handler executions.",
			},
			[]string{"command_name", "status"},
		),
		QueryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "cqrs_query_duration_seconds",
				Help:    "Duration of query handler execution in seconds.",
				Buckets: defaultBuckets,
			},
			[]string{"query_name", "status"},
		),
		QueryTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cqrs_query_total",
				Help: "Total number of query handler executions.",
			},
			[]string{"query_name", "status"},
		),
	}

	prometheus.MustRegister(
		m.HTTPRequestDuration,
		m.HTTPRequestTotal,
		m.HTTPRequestsInFlight,
		m.GRPCRequestDuration,
		m.GRPCRequestTotal,
		m.DBQueryDuration,
		m.OutboxQueueDepth,
		m.OutboxPublishTotal,
		m.OutboxPolledTotal,
		m.OutboxDLQTotal,
		m.OutboxDLQInsertErrors,
		m.OutboxMarkPublishedErrors,
		m.OutboxPollDuration,
		m.EventPublishTotal,
		m.EventConsumeTotal,
		m.EventConsumeDuration,
		m.QueryCacheHits,
		m.QueryCacheMisses,
		m.CommandDuration,
		m.CommandTotal,
		m.QueryDuration,
		m.QueryTotal,
	)

	return m
}

// Reset clears the singleton for testing. Not safe for concurrent use.
func Reset() {
	singletonOnce = sync.Once{}
	singleton = nil
}
