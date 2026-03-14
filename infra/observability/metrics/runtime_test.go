package metrics_test

import (
	"runtime"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	prommodel "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/observability/metrics"
)

func TestRegisterRuntimeMetrics_GoroutineGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics.RegisterRuntimeMetrics(reg)

	families, err := reg.Gather()
	require.NoError(t, err)

	found := metricByName(families, "wolf_runtime_goroutines_active")
	require.NotNil(t, found, "wolf_runtime_goroutines_active must be registered")

	value := found.GetMetric()[0].GetGauge().GetValue()
	assert.Greater(t, value, float64(0), "goroutine count must be positive")
	assert.LessOrEqual(t, value, float64(runtime.NumGoroutine()+100))
}

func TestRegisterRuntimeMetrics_GOMAXPROCSGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics.RegisterRuntimeMetrics(reg)

	families, err := reg.Gather()
	require.NoError(t, err)

	found := metricByName(families, "wolf_runtime_gomaxprocs")
	require.NotNil(t, found, "wolf_runtime_gomaxprocs must be registered")

	value := found.GetMetric()[0].GetGauge().GetValue()
	assert.Equal(t, float64(runtime.GOMAXPROCS(0)), value)
}

func metricByName(families []*prommodel.MetricFamily, name string) *prommodel.MetricFamily {
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}
