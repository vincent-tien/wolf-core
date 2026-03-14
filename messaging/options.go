// options.go — Consumer configuration for stream subscriptions (group, durable, retry).
package messaging

import "time"

// SubscribeConfig holds the resolved consumer configuration after applying all
// SubscribeOption functions. Zero values carry sensible defaults; adapters
// decide how to interpret unset fields.
type SubscribeConfig struct {
	// Group is the consumer group / queue group name for competing consumers.
	// An empty string means the subscription is not part of a group.
	Group string
	// Durable is the durable consumer name for persistent subscriptions that
	// survive consumer restarts. An empty string means ephemeral.
	Durable string
	// MaxDeliver is the maximum number of delivery attempts before a message is
	// moved to the dead-letter queue. 0 means unlimited.
	MaxDeliver int
	// MaxAckPending is the maximum number of unacknowledged messages the broker
	// will deliver to this consumer at once. 0 means the adapter default.
	MaxAckPending int
	// AckWait is the duration the broker waits for an ack before redelivering.
	// 0 means the adapter default.
	AckWait time.Duration
}

// SubscribeOption is a functional option that mutates a SubscribeConfig.
type SubscribeOption func(*SubscribeConfig)

// WithGroup sets the consumer group (queue group) name.
func WithGroup(group string) SubscribeOption {
	return func(c *SubscribeConfig) { c.Group = group }
}

// WithDurable sets the durable consumer name, enabling persistent subscriptions.
func WithDurable(name string) SubscribeOption {
	return func(c *SubscribeConfig) { c.Durable = name }
}

// WithMaxDeliver sets the maximum delivery attempts before dead-lettering.
func WithMaxDeliver(n int) SubscribeOption {
	return func(c *SubscribeConfig) { c.MaxDeliver = n }
}

// WithMaxAckPending sets the maximum number of unacknowledged messages in flight.
func WithMaxAckPending(n int) SubscribeOption {
	return func(c *SubscribeConfig) { c.MaxAckPending = n }
}

// WithAckWait sets the ack wait timeout.
func WithAckWait(d time.Duration) SubscribeOption {
	return func(c *SubscribeConfig) { c.AckWait = d }
}

// ApplyOpts constructs a SubscribeConfig from the given options.
// It is used by Stream adapters to resolve final configuration before passing
// consumer parameters to the underlying broker client.
func ApplyOpts(opts ...SubscribeOption) SubscribeConfig {
	cfg := SubscribeConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}
