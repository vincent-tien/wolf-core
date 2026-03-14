package decorator_test

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/cache"
	"github.com/vincent-tien/wolf-core/infra/decorator"
)

// Example_stackedDecorators shows how to compose multiple middlewares around a
// repository function. Execution order (outermost first): Logging → Cache → Metrics.
func Example_stackedDecorators() {
	logger := zap.NewNop()

	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "repo_duration_seconds", Help: "duration", Buckets: prometheus.DefBuckets},
		[]string{"operation", "status"},
	)
	total := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "repo_total", Help: "total"},
		[]string{"operation", "status"},
	)

	stubClient := &exampleCache{}

	repoGetByID := decorator.Func[string, string](func(_ context.Context, id string) (string, error) {
		return "product:" + id, nil
	})

	decorated := decorator.Chain(
		repoGetByID,
		decorator.WithLogging[string, string](logger, "GetByID"),
		decorator.WithCache[string, string](stubClient, func(id string) string { return "product:" + id }, time.Minute, logger),
		decorator.WithMetrics[string, string](duration, total, "GetByID"),
	)

	result, err := decorated(context.Background(), "42")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(result)
	// Output: product:42
}

// exampleCache is a minimal cache.Client stub for the example.
type exampleCache struct{}

func (e *exampleCache) Get(_ context.Context, _ string) ([]byte, error)            { return nil, cache.ErrCacheMiss }
func (e *exampleCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error { return nil }
func (e *exampleCache) Delete(_ context.Context, _ ...string) error                { return nil }
func (e *exampleCache) Ping(_ context.Context) error                               { return nil }
func (e *exampleCache) Close() error                                               { return nil }
