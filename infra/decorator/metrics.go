// metrics.go — CQRS middleware that records Prometheus histograms per handler.
package decorator

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// WithMetrics returns a Middleware that observes execution duration on the
// provided HistogramVec and increments the CounterVec after each call.
// Both collectors receive "operation" and "status" labels where status is
// "success" when err == nil and "error" otherwise.
func WithMetrics[In, Out any](
	duration *prometheus.HistogramVec,
	total *prometheus.CounterVec,
	operation string,
) Middleware[In, Out] {
	return func(next Func[In, Out]) Func[In, Out] {
		return func(ctx context.Context, in In) (Out, error) {
			start := time.Now()

			result, err := next(ctx, in)

			status := "success"
			if err != nil {
				status = "error"
			}

			elapsed := time.Since(start).Seconds()
			duration.WithLabelValues(operation, status).Observe(elapsed)
			total.WithLabelValues(operation, status).Inc()

			return result, err
		}
	}
}
