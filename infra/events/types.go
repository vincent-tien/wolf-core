// Package events provides the event bus factory and driver constants for the
// wolf-be platform. It wires together in-process and broker-backed
// implementations behind a single event.Bus interface.
package events

import "github.com/vincent-tien/wolf-core/infra/config"

// Driver identifies the underlying event transport implementation.
type Driver string

const (
	DriverInProcess Driver = Driver(config.BrokerInProcess)
	DriverKafka     Driver = Driver(config.BrokerKafka)
	DriverRabbitMQ  Driver = Driver(config.BrokerRabbitMQ)
	DriverNATS      Driver = Driver(config.BrokerNATS)
)
