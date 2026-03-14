// Package memory provides a channel-based in-memory transport for testing and development.
package memory

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/transport"
)

const defaultBufferSize = 1000

var _ transport.Transport = (*Transport)(nil)

// Transport is a channel-based in-memory transport.
type Transport struct {
	name    string
	ch      chan messenger.Envelope
	pending sync.Map // envelope MessageTypeName+createdAt → Envelope
	closed  atomic.Bool
}

// Option configures a memory transport.
type Option func(*Transport)

// WithBufferSize sets the channel buffer size.
func WithBufferSize(size int) Option {
	return func(t *Transport) {
		t.ch = make(chan messenger.Envelope, size)
	}
}

// WithName sets the transport name.
func WithName(name string) Option {
	return func(t *Transport) {
		t.name = name
	}
}

// New creates a memory transport with the given options.
func New(opts ...Option) *Transport {
	t := &Transport{
		name: "memory",
		ch:   make(chan messenger.Envelope, defaultBufferSize),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *Transport) Name() string { return t.name }

var pendingSeq atomic.Uint64

// memoryIDStamp is a private stamp for unique pending-map keys.
type memoryIDStamp struct{ ID string }

func (memoryIDStamp) StampName() string { return "memory.internal_id" }

// Send writes an envelope to the channel buffer.
func (t *Transport) Send(_ context.Context, env messenger.Envelope) error {
	if t.closed.Load() {
		return fmt.Errorf("transport: %s is closed", t.name)
	}
	env = env.WithStamp(memoryIDStamp{ID: strconv.FormatUint(pendingSeq.Add(1), 10)})
	t.ch <- env
	return nil
}

// Get reads envelopes from the channel. Returns up to 1 envelope per call.
// Blocks until an envelope is available or context is cancelled.
func (t *Transport) Get(ctx context.Context) ([]messenger.Envelope, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case env, ok := <-t.ch:
		if !ok {
			return nil, nil
		}
		key := pendingKey(env)
		t.pending.Store(key, env)
		return []messenger.Envelope{env}, nil
	}
}

// Ack removes the envelope from the pending set.
func (t *Transport) Ack(_ context.Context, env messenger.Envelope) error {
	key := pendingKey(env)
	t.pending.Delete(key)
	return nil
}

// Reject puts the envelope back in the channel for redelivery.
func (t *Transport) Reject(_ context.Context, env messenger.Envelope, _ error) error {
	key := pendingKey(env)
	t.pending.Delete(key)

	if t.closed.Load() {
		return fmt.Errorf("transport: %s is closed", t.name)
	}

	// Non-blocking requeue; drop if buffer is full.
	select {
	case t.ch <- env:
	default:
		return fmt.Errorf("transport: %s buffer full, message dropped on reject", t.name)
	}
	return nil
}

// Close closes the channel. Returns number of pending (unacked) messages.
func (t *Transport) Close() error {
	if t.closed.CompareAndSwap(false, true) {
		close(t.ch)
	}
	return nil
}

// Len returns the number of buffered messages (for testing).
func (t *Transport) Len() int {
	return len(t.ch)
}

func pendingKey(env messenger.Envelope) string {
	if s := env.Last("memory.internal_id"); s != nil {
		return s.(memoryIDStamp).ID
	}
	return fmt.Sprintf("%s:%d", env.MessageTypeName(), env.CreatedAt().UnixNano())
}

// Factory creates memory transports from "memory://" DSN strings.
type Factory struct{}

func (Factory) Supports(dsn string) bool {
	return strings.HasPrefix(dsn, "memory://")
}

func (Factory) Create(_ string, opts map[string]any) (transport.Transport, error) {
	var mopts []Option
	if size, ok := opts["buffer_size"].(int); ok {
		mopts = append(mopts, WithBufferSize(size))
	}
	if name, ok := opts["name"].(string); ok {
		mopts = append(mopts, WithName(name))
	}
	return New(mopts...), nil
}
