package decorator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/decorator"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newDecoratorRegistry(t *testing.T) (*prometheus.Registry, *prometheus.HistogramVec, *prometheus.CounterVec) {
	t.Helper()

	reg := prometheus.NewRegistry()

	dur := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dec_duration_seconds",
			Help:    "test",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)
	cnt := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dec_total",
			Help: "test",
		},
		[]string{"operation", "status"},
	)

	reg.MustRegister(dur, cnt)
	return reg, dur, cnt
}

func readCounter(t *testing.T, cv *prometheus.CounterVec, labels prometheus.Labels) float64 {
	t.Helper()
	m := &dto.Metric{}
	require.NoError(t, cv.With(labels).Write(m))
	return m.GetCounter().GetValue()
}

func histogramSampleCount(t *testing.T, reg *prometheus.Registry, name string) uint64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)

	var count uint64
	for _, fam := range families {
		if fam.GetName() != name {
			continue
		}
		for _, m := range fam.GetMetric() {
			count += m.GetHistogram().GetSampleCount()
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestWithMetrics_SuccessObserved(t *testing.T) {
	t.Parallel()

	// Arrange
	reg, dur, cnt := newDecoratorRegistry(t)
	fn := decorator.Func[string, string](func(_ context.Context, in string) (string, error) {
		return in, nil
	})
	wrapped := decorator.Chain(fn, decorator.WithMetrics[string, string](dur, cnt, "GetProduct"))

	// Act
	_, err := wrapped(context.Background(), "x")

	// Assert
	require.NoError(t, err)

	v := readCounter(t, cnt, prometheus.Labels{"operation": "GetProduct", "status": "success"})
	assert.Equal(t, float64(1), v)

	sc := histogramSampleCount(t, reg, "dec_duration_seconds")
	assert.Equal(t, uint64(1), sc, "histogram must have one observation")
}

func TestWithMetrics_ErrorObserved(t *testing.T) {
	t.Parallel()

	// Arrange
	_, dur, cnt := newDecoratorRegistry(t)
	sentinel := errors.New("db error")
	fn := decorator.Func[string, string](func(_ context.Context, _ string) (string, error) {
		return "", sentinel
	})
	wrapped := decorator.Chain(fn, decorator.WithMetrics[string, string](dur, cnt, "GetProduct"))

	// Act
	_, err := wrapped(context.Background(), "x")

	// Assert
	require.ErrorIs(t, err, sentinel)

	v := readCounter(t, cnt, prometheus.Labels{"operation": "GetProduct", "status": "error"})
	assert.Equal(t, float64(1), v)
}
