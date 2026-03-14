// Package nats provides a NATS JetStream implementation of messaging.Stream.
// It uses the new github.com/nats-io/nats.go/jetstream API (not the legacy
// nats.JetStreamContext) for durable pull-consumer subscriptions with
// explicit acknowledgement.
package nats

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/messaging"
)

// Config holds the parameters required to connect to NATS and bind a
// JetStream stream. All fields except Replicas are required at runtime.
type Config struct {
	// URL is the NATS server connection URL (e.g. "nats://localhost:4222").
	URL string
	// Stream is the JetStream stream name that will be created or updated.
	Stream string
	// Subjects is the list of NATS subjects the stream will listen on.
	// At least one subject must be provided.
	Subjects []string
	// Replicas is the number of stream replicas for clustered JetStream.
	// A value of 0 is normalised to 1 (single replica).
	Replicas int
}

// Bus is a NATS JetStream backed implementation of messaging.Stream.
// It creates or updates a JetStream stream on startup and supports durable
// pull consumers via Subscribe. Published messages use WithMsgID for
// server-side deduplication.
//
// Bus is safe for concurrent use.
type Bus struct {
	nc        *nats.Conn
	js        jetstream.JetStream
	stream    jetstream.Stream
	mu        sync.Mutex
	consumers []jetstream.ConsumeContext
	logger    *zap.Logger
}

// NewBus connects to NATS at cfg.URL, obtains a JetStream context, and
// creates or updates the stream named cfg.Stream bound to cfg.Subjects.
// An error is returned if the connection fails or the stream cannot be
// provisioned. Callers must call Close when the Bus is no longer needed.
func NewBus(cfg Config, logger *zap.Logger) (*Bus, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("nats: URL must not be empty")
	}
	if cfg.Stream == "" {
		return nil, fmt.Errorf("nats: Stream name must not be empty")
	}
	if len(cfg.Subjects) == 0 {
		return nil, fmt.Errorf("nats: at least one Subject must be provided")
	}

	replicas := cfg.Replicas
	if replicas <= 0 {
		replicas = 1
	}

	nc, err := nats.Connect(cfg.URL,
		nats.Name("wolf-be"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("nats: connect to %s: %w", cfg.URL, err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats: create JetStream context: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      cfg.Stream,
		Subjects:  cfg.Subjects,
		Replicas:  replicas,
		Storage:   jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats: create or update stream %q: %w", cfg.Stream, err)
	}

	logger.Info("nats: JetStream stream ready",
		zap.String("stream", cfg.Stream),
		zap.Strings("subjects", cfg.Subjects),
		zap.Int("replicas", replicas),
	)

	return &Bus{
		nc:     nc,
		js:     js,
		stream: stream,
		logger: logger,
	}, nil
}

// Publish sends msg on the given subject. Headers from msg.Headers are
// forwarded as NATS message headers. If msg.ID is non-empty it is set as
// the NATS deduplication message ID via WithMsgID.
//
// The subject parameter takes precedence over msg.Subject, consistent with
// the messaging.Publisher contract.
func (b *Bus) Publish(ctx context.Context, subject string, msg messaging.RawMessage) error {
	natMsg := &nats.Msg{
		Subject: subject,
		Data:    msg.Data,
		Header:  make(nats.Header, len(msg.Headers)),
	}

	// Copy caller-provided headers verbatim.
	for k, v := range msg.Headers {
		natMsg.Header.Set(k, v)
	}

	opts := make([]jetstream.PublishOpt, 0, 1)
	if msg.ID != "" {
		opts = append(opts, jetstream.WithMsgID(msg.ID))
	}

	if _, err := b.js.PublishMsg(ctx, natMsg, opts...); err != nil {
		return fmt.Errorf("nats: publish to subject %q: %w", subject, err)
	}
	return nil
}

// Subscribe creates a JetStream pull consumer for subject and calls handler
// for every received message. The consumer is durable when opts includes
// WithDurable; otherwise an ephemeral consumer is created.
//
// Each message is handled in a dedicated goroutine. Handler errors cause
// the message to be negatively acknowledged so that NATS retries delivery
// according to the consumer's MaxDeliver setting.
func (b *Bus) Subscribe(subject string, handler messaging.MessageHandler, opts ...messaging.SubscribeOption) error {
	cfg := messaging.ApplyOpts(opts...)

	ackWait := cfg.AckWait
	if ackWait <= 0 {
		ackWait = 30 * time.Second
	}

	maxDeliver := cfg.MaxDeliver
	if maxDeliver <= 0 {
		maxDeliver = -1 // unlimited
	}

	maxAckPending := cfg.MaxAckPending
	if maxAckPending <= 0 {
		maxAckPending = 1000
	}

	consCfg := jetstream.ConsumerConfig{
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		MaxDeliver:    maxDeliver,
		MaxAckPending: maxAckPending,
	}

	if cfg.Durable != "" {
		consCfg.Durable = cfg.Durable
		consCfg.Name = cfg.Durable
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	consumer, err := b.stream.CreateOrUpdateConsumer(ctx, consCfg)
	if err != nil {
		return fmt.Errorf("nats: create consumer for subject %q: %w", subject, err)
	}

	// Use ackWait as the handler timeout — the handler must complete before
	// the broker considers the message unacknowledged.
	handlerTimeout := ackWait
	consumeCtx, err := consumer.Consume(func(jsMsg jetstream.Msg) {
		m := &natsMessage{msg: jsMsg}
		handlerCtx, handlerCancel := context.WithTimeout(context.Background(), handlerTimeout)
		defer handlerCancel()
		if handlerErr := handler(handlerCtx, m); handlerErr != nil {
			b.logger.Error("nats: message handler error",
				zap.String("subject", jsMsg.Subject()),
				zap.Error(handlerErr),
			)
			if nakErr := jsMsg.Nak(); nakErr != nil {
				b.logger.Warn("nats: nak failed", zap.Error(nakErr))
			}
			return
		}
		if ackErr := jsMsg.Ack(); ackErr != nil {
			b.logger.Warn("nats: ack failed", zap.Error(ackErr))
		}
	})
	if err != nil {
		return fmt.Errorf("nats: start consume for subject %q: %w", subject, err)
	}

	b.mu.Lock()
	b.consumers = append(b.consumers, consumeCtx)
	b.mu.Unlock()

	b.logger.Info("nats: subscribed",
		zap.String("subject", subject),
		zap.String("durable", cfg.Durable),
	)

	return nil
}

// HealthCheck verifies the NATS connection is alive. Returns nil when
// connected, or an error when the connection is closed or reconnecting.
func (b *Bus) HealthCheck(_ context.Context) error {
	if !b.nc.IsConnected() {
		return fmt.Errorf("nats: connection status %s", b.nc.Status())
	}
	return nil
}

// Close stops all active consume contexts and drains the NATS connection,
// waiting for in-flight messages to complete. It is safe to call Close
// multiple times.
func (b *Bus) Close() error {
	b.mu.Lock()
	consumers := make([]jetstream.ConsumeContext, len(b.consumers))
	copy(consumers, b.consumers)
	b.consumers = nil
	b.mu.Unlock()

	for _, cc := range consumers {
		cc.Stop()
	}

	if err := b.nc.Drain(); err != nil {
		return fmt.Errorf("nats: drain connection: %w", err)
	}

	b.logger.Info("nats: bus closed")
	return nil
}

// natsMessage adapts a jetstream.Msg to the messaging.Message interface.
type natsMessage struct {
	msg jetstream.Msg
}

// ID returns the NATS deduplication message ID stored in the Nats-Msg-Id
// header. Returns an empty string when no ID was set at publish time.
func (m *natsMessage) ID() string {
	return m.msg.Headers().Get(jetstream.MsgIDHeader)
}

// Subject returns the NATS subject the message was published on.
func (m *natsMessage) Subject() string {
	return m.msg.Subject()
}

// Data returns the raw message payload bytes.
func (m *natsMessage) Data() []byte {
	return m.msg.Data()
}

// Headers converts the NATS nats.Header (a map[string][]string) to a flat
// map[string]string using the first value for each header key.
func (m *natsMessage) Headers() map[string]string {
	natsHeaders := m.msg.Headers()
	if len(natsHeaders) == 0 {
		return nil
	}
	result := make(map[string]string, len(natsHeaders))
	for k, vals := range natsHeaders {
		if len(vals) > 0 {
			result[k] = vals[0]
		}
	}
	return result
}

// Ack positively acknowledges the message, signalling successful processing.
func (m *natsMessage) Ack() error {
	return m.msg.Ack()
}

// Nak negatively acknowledges the message, requesting immediate redelivery.
func (m *natsMessage) Nak() error {
	return m.msg.Nak()
}

// NakWithDelay negatively acknowledges the message and requests redelivery
// after the specified delay.
func (m *natsMessage) NakWithDelay(d time.Duration) error {
	return m.msg.NakWithDelay(d)
}

// Term terminates the message without requesting redelivery. The message
// is discarded by the broker.
func (m *natsMessage) Term() error {
	return m.msg.Term()
}

// DeliveryAttempt returns the 1-based number of times this message has been
// delivered. The value is read from the JetStream message metadata. Returns 1
// when the metadata cannot be parsed.
func (m *natsMessage) DeliveryAttempt() int {
	meta, err := m.msg.Metadata()
	if err != nil {
		// Fall back to the header-based value for safety.
		raw := m.msg.Headers().Get("Nats-Num-Delivered")
		if raw == "" {
			return 1
		}
		n, parseErr := strconv.Atoi(raw)
		if parseErr != nil || n < 1 {
			return 1
		}
		return n
	}
	n := int(meta.NumDelivered)
	if n < 1 {
		return 1
	}
	return n
}
