// factory.go — Creates event bus and messaging stream based on config driver (inprocess/nats/kafka/rabbitmq).
package events

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/config"
	"github.com/vincent-tien/wolf-core/infra/events/inprocess"
	natsBus "github.com/vincent-tien/wolf-core/infra/events/nats"
	"github.com/vincent-tien/wolf-core/event"
	"github.com/vincent-tien/wolf-core/messaging"
)

// NewBus constructs an event.Bus for the given driver string.
//
// The event.Bus is always in-process: domain events are dispatched synchronously
// within the running process. External event delivery is handled by the outbox →
// stream pipeline (see NewStream). When the broker driver is nats/kafka/rabbitmq,
// the bus still uses in-process dispatch — only the stream changes transport.
func NewBus(driver string, logger *zap.Logger) (event.Bus, error) {
	switch Driver(driver) {
	case DriverInProcess, DriverNATS, DriverKafka, DriverRabbitMQ:
		return inprocess.NewBus(logger), nil
	default:
		return nil, fmt.Errorf("events: unknown driver %q", driver)
	}
}

// StreamResult bundles the messaging.Stream with an optional health check
// function for broker readiness probes.
type StreamResult struct {
	Stream      messaging.Stream
	HealthCheck func(ctx context.Context) error
}

// StreamConfigs groups the per-driver broker configurations needed by
// NewStream. Each driver reads only its own config; the others are ignored.
// Accepting all configs upfront means adding a new driver never changes the
// function signature.
type StreamConfigs struct {
	NATS     config.NATSConfig
	Kafka    config.KafkaConfig
	RabbitMQ config.RabbitMQConfig
}

// NewStream constructs a messaging.Stream for the given driver string.
// It also returns an optional health check function for readiness probes.
//
// Supported drivers:
//   - "inprocess" – goroutine-safe in-memory delivery, suitable for
//     development and testing. No broker configuration is required.
//   - "nats" – NATS JetStream backed stream with durable pull consumers,
//     at-least-once delivery, and server-side deduplication via message IDs.
//
// An error is returned for unknown drivers so that misconfiguration is caught
// at startup rather than silently dropped.
func NewStream(driver string, cfgs StreamConfigs, logger *zap.Logger) (*StreamResult, error) {
	switch Driver(driver) {
	case DriverInProcess:
		return &StreamResult{Stream: inprocess.NewStream(logger)}, nil
	case DriverNATS:
		bus, err := natsBus.NewBus(natsBus.Config{
			URL:      cfgs.NATS.URL,
			Stream:   cfgs.NATS.Stream,
			Subjects: cfgs.NATS.Subjects,
			Replicas: cfgs.NATS.Replicas,
		}, logger)
		if err != nil {
			return nil, err
		}
		return &StreamResult{Stream: bus, HealthCheck: bus.HealthCheck}, nil
	default:
		return nil, fmt.Errorf("events: unsupported stream driver %q", driver)
	}
}
