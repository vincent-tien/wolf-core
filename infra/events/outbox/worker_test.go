package outbox_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/events/deadletter"
	"github.com/vincent-tien/wolf-core/infra/events/outbox"
	"github.com/vincent-tien/wolf-core/event"
)

// --- mocks -------------------------------------------------------------------

type mockStore struct {
	mu           sync.Mutex
	entries      []outbox.OutboxEntry
	claimed      map[string]bool     // entries claimed by ClaimBatch
	released     map[string][]string // id -> list of lastError values from ReleaseClaim
}

func newMockStore(entries ...outbox.OutboxEntry) *mockStore {
	return &mockStore{
		entries:  entries,
		claimed:  make(map[string]bool),
		released: make(map[string][]string),
	}
}

func (s *mockStore) ClaimBatch(_ context.Context, _ int) ([]outbox.OutboxEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []outbox.OutboxEntry
	for _, e := range s.entries {
		if !s.claimed[e.ID] {
			result = append(result, e)
			s.claimed[e.ID] = true
		}
	}
	return result, nil
}

func (s *mockStore) ReleaseClaim(_ context.Context, id, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseLocked(id, lastError)
	return nil
}

func (s *mockStore) ReleaseClaims(_ context.Context, entries []outbox.ReleaseEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		s.releaseLocked(e.ID, e.LastError)
	}
	return nil
}

func (s *mockStore) releaseLocked(id, lastError string) {
	s.released[id] = append(s.released[id], lastError)
	delete(s.claimed, id)
	for i := range s.entries {
		if s.entries[i].ID == id {
			s.entries[i].RetryCount++
			s.entries[i].LastError = lastError
		}
	}
}

func (s *mockStore) Cleanup(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

func (s *mockStore) MarkPublished(_ context.Context, ids []string, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Mark entries as published (update internal state to reflect published_at = NOW())
	// but keep them claimed so we can verify they remain claimed after publish.
	for _, id := range ids {
		for i := range s.entries {
			if s.entries[i].ID == id {
				now := time.Now()
				s.entries[i].PublishedAt = &now
			}
		}
	}
	return nil
}

func (s *mockStore) CountPending(_ context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var count int64
	for _, e := range s.entries {
		if !s.claimed[e.ID] {
			count++
		}
	}
	return count, nil
}

type mockPublisher struct {
	mu        sync.Mutex
	published []string
	err       error
}

func (p *mockPublisher) Publish(_ context.Context, evt event.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, evt.EventID())
	return nil
}

// --- helpers -----------------------------------------------------------------

func makeEntry(id string, retries int) outbox.OutboxEntry {
	return outbox.OutboxEntry{
		ID:            id,
		AggregateType: "test",
		AggregateID:   "agg-1",
		EventType:     "test.event.v1",
		Payload:       []byte(`{}`),
		CreatedAt:     time.Now(),
		RetryCount:    retries,
	}
}

func startAndWait(t *testing.T, w *outbox.Worker) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := w.Start(ctx)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		require.NoError(t, err)
	}
}

// --- tests -------------------------------------------------------------------

func TestWorker_ReleaseClaim_OnPublishFailure(t *testing.T) {
	store := newMockStore(makeEntry("evt-1", 0))
	pub := &mockPublisher{err: errors.New("broker down")}

	// maxRetries=100 ensures the entry won't exhaust across multiple poll cycles.
	w := outbox.NewWorker(store, pub, 50*time.Millisecond, 100, 100, 24*time.Hour, zap.NewNop(), nil).
		WithPollTimeout(5 * time.Second)

	startAndWait(t, w)

	store.mu.Lock()
	defer store.mu.Unlock()

	require.NotEmpty(t, store.released["evt-1"], "claim must be released after publish failure")
	assert.Contains(t, store.released["evt-1"][0], "broker down")
}

func TestWorker_NoRelease_OnSuccess(t *testing.T) {
	store := newMockStore(makeEntry("evt-1", 0))
	pub := &mockPublisher{}

	w := outbox.NewWorker(store, pub, 50*time.Millisecond, 100, 3, 24*time.Hour, zap.NewNop(), nil).
		WithPollTimeout(5 * time.Second)

	startAndWait(t, w)

	store.mu.Lock()
	defer store.mu.Unlock()

	assert.Empty(t, store.released["evt-1"], "claim must NOT be released on success")
	assert.True(t, store.claimed["evt-1"], "entry must remain claimed after successful publish")
}

func TestWorker_CircuitBreakerOpen_FailsPublish(t *testing.T) {
	store := newMockStore(makeEntry("evt-1", 0))
	pub := &mockPublisher{}

	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "test",
		MaxRequests: 1,
		Interval:    10 * time.Minute,
		Timeout:     10 * time.Minute,
		ReadyToTrip: func(_ gobreaker.Counts) bool { return true },
	})

	// Trip the breaker by executing a failing call.
	_, _ = cb.Execute(func() (any, error) {
		return nil, errors.New("fail")
	})

	w := outbox.NewWorker(store, pub, 50*time.Millisecond, 100, 100, 24*time.Hour, zap.NewNop(), nil).
		WithPollTimeout(5 * time.Second).
		WithCircuitBreaker(cb)

	startAndWait(t, w)

	pub.mu.Lock()
	defer pub.mu.Unlock()

	assert.Empty(t, pub.published, "publish must be blocked when circuit breaker is open")

	store.mu.Lock()
	defer store.mu.Unlock()

	assert.NotEmpty(t, store.released["evt-1"], "claim should be released because CB error counts as publish failure")
}

func TestWorker_ExhaustedEntry_StaysClaimed(t *testing.T) {
	store := newMockStore(makeEntry("evt-1", 5))
	pub := &mockPublisher{}

	w := outbox.NewWorker(store, pub, 50*time.Millisecond, 100, 5, 24*time.Hour, zap.NewNop(), nil).
		WithPollTimeout(5 * time.Second)

	startAndWait(t, w)

	store.mu.Lock()
	defer store.mu.Unlock()

	assert.True(t, store.claimed["evt-1"], "exhausted entry must stay claimed (not re-polled)")

	pub.mu.Lock()
	defer pub.mu.Unlock()

	assert.Empty(t, pub.published, "exhausted entry must not be published to broker")
}

func TestWorker_MixedBatch(t *testing.T) {
	store := newMockStore(
		makeEntry("ok-1", 0),
		makeEntry("fail-1", 0),
		makeEntry("exhausted-1", 5),
	)
	selectivePub := &selectivePublisher{
		failIDs: map[string]error{"fail-1": errors.New("selective fail")},
	}

	w := outbox.NewWorker(store, selectivePub, 50*time.Millisecond, 100, 5, 24*time.Hour, zap.NewNop(), nil).
		WithPollTimeout(5 * time.Second)

	startAndWait(t, w)

	store.mu.Lock()
	defer store.mu.Unlock()

	assert.True(t, store.claimed["ok-1"], "successful entry must stay claimed")
	assert.True(t, store.claimed["exhausted-1"], "exhausted entry must stay claimed")
	assert.False(t, store.claimed["fail-1"], "failed entry must be released (unclaimed)")
	assert.NotEmpty(t, store.released["fail-1"], "failed entry must have release recorded")
}

func TestWorker_ExhaustedEntry_DLQFailure_ReleasesEntry(t *testing.T) {
	store := newMockStore(makeEntry("evt-1", 5))
	pub := &mockPublisher{}
	dlq := &failingDLQ{err: errors.New("table does not exist")}

	w := outbox.NewWorker(store, pub, 50*time.Millisecond, 100, 5, 24*time.Hour, zap.NewNop(), nil).
		WithPollTimeout(5 * time.Second).
		WithDLQ(dlq)

	startAndWait(t, w)

	store.mu.Lock()
	defer store.mu.Unlock()

	// The entry must NOT be marked published — it should be released back
	// to the outbox so it can be retried on the next poll cycle.
	assert.False(t, store.claimed["evt-1"], "entry must be released when DLQ insert fails")
	assert.NotEmpty(t, store.released["evt-1"], "entry must have release recorded on DLQ failure")
	assert.Contains(t, store.released["evt-1"][0], "table does not exist")

	pub.mu.Lock()
	defer pub.mu.Unlock()

	assert.Empty(t, pub.published, "exhausted entry must not be published to broker")
}

func TestWorker_ExhaustedEntry_DLQSuccess_MarksPublished(t *testing.T) {
	store := newMockStore(makeEntry("evt-1", 5))
	pub := &mockPublisher{}
	dlq := &successDLQ{}

	w := outbox.NewWorker(store, pub, 50*time.Millisecond, 100, 5, 24*time.Hour, zap.NewNop(), nil).
		WithPollTimeout(5 * time.Second).
		WithDLQ(dlq)

	startAndWait(t, w)

	store.mu.Lock()
	defer store.mu.Unlock()

	assert.True(t, store.claimed["evt-1"], "entry must stay claimed (marked published) after DLQ success")
	require.NotNil(t, store.entries[0].PublishedAt, "entry must be marked published after DLQ success")

	assert.True(t, dlq.inserted, "DLQ insert must have been called")
}

// --- DLQ mocks ---------------------------------------------------------------

type failingDLQ struct {
	err error
}

func (d *failingDLQ) Insert(_ context.Context, _ deadletter.DLQEntry) error {
	return d.err
}

type successDLQ struct {
	inserted bool
}

func (d *successDLQ) Insert(_ context.Context, _ deadletter.DLQEntry) error {
	d.inserted = true
	return nil
}

// selectivePublisher fails only for specific event IDs.
type selectivePublisher struct {
	mu      sync.Mutex
	failIDs map[string]error
}

func (p *selectivePublisher) Publish(_ context.Context, evt event.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err, ok := p.failIDs[evt.EventID()]; ok {
		return err
	}
	return nil
}
