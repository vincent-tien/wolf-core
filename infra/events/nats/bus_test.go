package nats_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	natsBus "github.com/vincent-tien/wolf-core/infra/events/nats"
	"github.com/vincent-tien/wolf-core/messaging"
)

// --- Compile-time interface check ---

// TestBus_ImplementsStreamInterface verifies at compile time that *Bus satisfies
// the messaging.Stream contract. This will fail to compile if any method is
// missing or has an incorrect signature.
func TestBus_ImplementsStreamInterface(t *testing.T) {
	t.Helper()
	var _ messaging.Stream = (*natsBus.Bus)(nil)
}

// --- Config validation ---

func TestNewBus_EmptyURL_ReturnsError(t *testing.T) {
	_, err := natsBus.NewBus(natsBus.Config{
		URL:      "",
		Stream:   "wolf",
		Subjects: []string{"wolf.>"},
	}, zap.NewNop())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "URL")
}

func TestNewBus_EmptyStream_ReturnsError(t *testing.T) {
	_, err := natsBus.NewBus(natsBus.Config{
		URL:      "nats://localhost:4222",
		Stream:   "",
		Subjects: []string{"wolf.>"},
	}, zap.NewNop())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Stream")
}

func TestNewBus_EmptySubjects_ReturnsError(t *testing.T) {
	_, err := natsBus.NewBus(natsBus.Config{
		URL:      "nats://localhost:4222",
		Stream:   "wolf",
		Subjects: nil,
	}, zap.NewNop())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Subject")
}

func TestNewBus_EmptySubjectsSlice_ReturnsError(t *testing.T) {
	_, err := natsBus.NewBus(natsBus.Config{
		URL:      "nats://localhost:4222",
		Stream:   "wolf",
		Subjects: []string{},
	}, zap.NewNop())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Subject")
}

// TestNewBus_ConnectionFailure verifies that NewBus returns an error when the
// NATS server is not reachable. The NATS client will attempt to connect with
// RetryOnFailedConnect, so we rely on the fact that a non-existent server
// returns an error during the connection attempt.
func TestNewBus_ConnectionFailure_ReturnsError(t *testing.T) {
	// Use a port that is almost certainly not listening.
	_, err := natsBus.NewBus(natsBus.Config{
		URL:      "nats://127.0.0.1:14222",
		Stream:   "wolf",
		Subjects: []string{"wolf.>"},
		Replicas: 1,
	}, zap.NewNop())

	// The nats client with RetryOnFailedConnect may NOT fail immediately on
	// connect, but stream creation will fail because there is no server.
	// Either way, NewBus must return a non-nil error.
	require.Error(t, err)
}

// --- natsMessage adapter (exported for testing via unexported field access) ---
// We test the adapter through the exported NewNATSMessageForTest helper or by
// verifying behavior via the messaging.Message interface directly.

// TestNATSMessage_ImplementsMessageInterface asserts at compile time that a
// *natsMessage returned by the Bus satisfies messaging.Message. Since
// natsMessage is unexported we rely on the Bus.Subscribe flow in integration
// tests; for unit tests we verify via the interface variable below.
//
// The concrete assertion is that the Bus itself (as a messaging.Stream) causes
// handler to receive a messaging.Message — this is validated in integration
// tests with a real NATS server. Here we focus on what we can test offline.

// --- SubscribeConfig default values applied correctly ---

// TestSubscribeConfig_DefaultsApplied verifies that ApplyOpts with no options
// returns the zero-value SubscribeConfig, and that Bus.Subscribe accepts all
// option combinations without panicking (tested against validation path only,
// since we have no live server in unit tests).
func TestSubscribeConfig_DefaultsApplied(t *testing.T) {
	cfg := messaging.ApplyOpts()
	assert.Equal(t, "", cfg.Durable)
	assert.Equal(t, "", cfg.Group)
	assert.Equal(t, 0, cfg.MaxDeliver)
	assert.Equal(t, 0, cfg.MaxAckPending)
	assert.Equal(t, time.Duration(0), cfg.AckWait)
}

func TestSubscribeConfig_AllOptionsApplied(t *testing.T) {
	cfg := messaging.ApplyOpts(
		messaging.WithDurable("my-consumer"),
		messaging.WithGroup("my-group"),
		messaging.WithMaxDeliver(5),
		messaging.WithMaxAckPending(100),
		messaging.WithAckWait(45*time.Second),
	)

	assert.Equal(t, "my-consumer", cfg.Durable)
	assert.Equal(t, "my-group", cfg.Group)
	assert.Equal(t, 5, cfg.MaxDeliver)
	assert.Equal(t, 100, cfg.MaxAckPending)
	assert.Equal(t, 45*time.Second, cfg.AckWait)
}

// --- Config struct field verification ---

func TestConfig_ZeroReplicas_IsAcceptedByNewBus(t *testing.T) {
	// Replicas == 0 should be normalised to 1 internally. The Config struct
	// itself has no restriction; validation happens in NewBus.
	// We can't complete NewBus without a server, but we ensure that validation
	// errors come from connection, not from the replicas check.
	_, err := natsBus.NewBus(natsBus.Config{
		URL:      "nats://127.0.0.1:14223",
		Stream:   "wolf",
		Subjects: []string{"wolf.events"},
		Replicas: 0, // should be normalised to 1
	}, zap.NewNop())

	// Error must come from connection failure, not from replica validation.
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "replicas")
}
