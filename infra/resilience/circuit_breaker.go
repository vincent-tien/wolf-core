// Package resilience provides reliability primitives for the wolf-be service,
// including circuit breakers and retry with exponential back-off.
package resilience

import (
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

// cbStateTransitions counts circuit breaker state transitions for alerting.
var cbStateTransitions = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "circuit_breaker_state_transitions_total",
		Help: "Total number of circuit breaker state transitions.",
	},
	[]string{"name", "from", "to"},
)

func init() {
	if err := prometheus.Register(cbStateTransitions); err != nil {
		// Tolerate duplicate registration in test binaries. Reuse the
		// already-registered collector so metrics are not silently lost.
		var are prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			cbStateTransitions = are.ExistingCollector.(*prometheus.CounterVec)
		}
	}
}

// NewCircuitBreaker constructs a *gobreaker.CircuitBreaker with sensible
// production defaults:
//   - ReadyToTrip fires when the error rate exceeds 50 % and at least one
//     request has been observed in the current window.
//   - State transitions are logged at info level via the provided logger.
//
// Parameters:
//
//	name        – human-readable identifier for the breaker (appears in logs).
//	maxRequests – number of requests allowed through while half-open.
//	interval    – duration of the closed-state counting window; 0 disables it.
//	timeout     – duration the breaker stays open before transitioning to half-open.
//	logger      – zap logger used for state-change notifications.
func NewCircuitBreaker(
	name string,
	maxRequests uint32,
	interval, timeout time.Duration,
	logger *zap.Logger,
) *gobreaker.CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        name,
		MaxRequests: maxRequests,
		Interval:    interval,
		Timeout:     timeout,

		// ReadyToTrip trips the breaker when more than 50 % of requests have
		// failed, as long as at least one request has been counted. This
		// prevents tripping on a single initial failure in a quiet window.
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests == 0 {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio > 0.5
		},

		// OnStateChange logs and increments a Prometheus counter on every
		// circuit-breaker state transition for alerting and correlation.
		OnStateChange: func(name string, from, to gobreaker.State) {
			logger.Info("circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
			cbStateTransitions.WithLabelValues(name, from.String(), to.String()).Inc()
		},
	}

	return gobreaker.NewCircuitBreaker(settings)
}
