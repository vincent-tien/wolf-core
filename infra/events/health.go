// health.go — Broker connection readiness probe for health/ready endpoints.
package events

import (
	"context"
	"fmt"
)

// BrokerHealthChecker provides a readiness probe for the underlying event
// broker. For in-process drivers the check always passes. For external brokers
// (NATS, Kafka, etc.) it verifies the connection is alive.
type BrokerHealthChecker struct {
	driver string
	check  func(ctx context.Context) error
}

// NewBrokerHealthChecker constructs a health checker for the given driver.
// checkFn may be nil for drivers that are always healthy (e.g. inprocess).
func NewBrokerHealthChecker(driver string, checkFn func(ctx context.Context) error) *BrokerHealthChecker {
	return &BrokerHealthChecker{
		driver: driver,
		check:  checkFn,
	}
}

// HealthCheck implements a readiness probe. Returns nil when the broker is
// reachable, or an error describing the connectivity issue.
func (b *BrokerHealthChecker) HealthCheck(ctx context.Context) error {
	if b.check == nil {
		return nil
	}

	if err := b.check(ctx); err != nil {
		return fmt.Errorf("broker(%s): %w", b.driver, err)
	}

	return nil
}
