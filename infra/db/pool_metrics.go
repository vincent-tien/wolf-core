// pool_metrics.go — Prometheus gauges for sql.DBStats (open/idle/in-use/wait per pool).
package db

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

// PoolMetrics exports sql.DBStats as Prometheus gauges. Call Register once at
// startup with the write and read pools. The Prometheus default registry
// scrapes the gauge funcs on every /metrics request — no background goroutine
// needed.
//
// Key metrics to monitor:
//   - db_pool_in_use: sustained at max_open_conns = pool exhaustion (503s incoming)
//   - db_pool_wait_count_total: rising rate = pool undersized, increase max_open_conns
//   - db_pool_wait_duration_seconds_total: >100ms avg wait = connection starvation
//
// All metrics are labeled with {pool="write"} or {pool="read"} for split monitoring.
type PoolMetrics struct {
	collectors []prometheus.Collector
}

// NewPoolMetrics creates gauge funcs for both write and read pools and
// registers them with the default Prometheus registry. Duplicate registrations
// (e.g. in test suites) are silently ignored.
func NewPoolMetrics(writeDB, readDB *sql.DB) *PoolMetrics {
	pm := &PoolMetrics{}
	pm.registerPool("write", writeDB)
	pm.registerPool("read", readDB)

	for _, c := range pm.collectors {
		// Ignore AlreadyRegisteredError — safe for tests and hot restarts.
		_ = prometheus.Register(c)
	}

	return pm
}

func (pm *PoolMetrics) registerPool(pool string, db *sql.DB) {
	gauges := []struct {
		name string
		help string
		fn   func() float64
	}{
		{
			name: "db_pool_open_connections",
			help: "Number of open connections (in-use + idle).",
			fn:   func() float64 { return float64(db.Stats().OpenConnections) },
		},
		{
			name: "db_pool_in_use",
			help: "Number of connections currently in use.",
			fn:   func() float64 { return float64(db.Stats().InUse) },
		},
		{
			name: "db_pool_idle",
			help: "Number of idle connections.",
			fn:   func() float64 { return float64(db.Stats().Idle) },
		},
		{
			name: "db_pool_wait_count_total",
			help: "Total number of connections waited for.",
			fn:   func() float64 { return float64(db.Stats().WaitCount) },
		},
		{
			name: "db_pool_wait_duration_seconds_total",
			help: "Total time blocked waiting for a new connection.",
			fn:   func() float64 { return db.Stats().WaitDuration.Seconds() },
		},
		{
			name: "db_pool_max_idle_closed_total",
			help: "Total connections closed due to SetMaxIdleConns.",
			fn:   func() float64 { return float64(db.Stats().MaxIdleClosed) },
		},
		{
			name: "db_pool_max_lifetime_closed_total",
			help: "Total connections closed due to SetConnMaxLifetime.",
			fn:   func() float64 { return float64(db.Stats().MaxLifetimeClosed) },
		},
	}

	for _, g := range gauges {
		gf := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name:        g.name,
			Help:        g.help,
			ConstLabels: prometheus.Labels{"pool": pool},
		}, g.fn)
		pm.collectors = append(pm.collectors, gf)
	}
}
