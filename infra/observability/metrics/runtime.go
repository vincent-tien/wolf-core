// runtime.go — Prometheus gauges for Go runtime stats (goroutines, memory, GC).
package metrics

import (
	"runtime"

	"github.com/prometheus/client_golang/prometheus"
)

// RegisterRuntimeMetrics exposes Go runtime metrics as Prometheus gauges.
// Goroutine count > 10 000 sustained for 5 minutes triggers the GoroutineLeak
// alert (see deployments/prometheus/alerts.rules.yml).
func RegisterRuntimeMetrics(reg prometheus.Registerer) {
	reg.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "wolf",
			Subsystem: "runtime",
			Name:      "goroutines_active",
			Help:      "Number of currently active goroutines.",
		}, func() float64 {
			return float64(runtime.NumGoroutine())
		}),

		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: "wolf",
			Subsystem: "runtime",
			Name:      "gomaxprocs",
			Help:      "Value of GOMAXPROCS.",
		}, func() float64 {
			return float64(runtime.GOMAXPROCS(0))
		}),
	)
}
