// Package uow_test contains unit tests for the UnitOfWork orchestration logic.
// Tests follow the table-driven AAA pattern and verify observable behaviour
// rather than implementation details.
package uow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/event"
	"github.com/vincent-tien/wolf-core/uow"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// mockTxRunner records whether RunInTx was called and executes fn immediately.
// Setting injectErr causes RunInTx to return that error without calling fn.
type mockTxRunner struct {
	called    bool
	injectErr error
}

func (m *mockTxRunner) RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	m.called = true
	if m.injectErr != nil {
		return m.injectErr
	}
	return fn(ctx)
}

// mockAggregate implements uow.Aggregate for testing.
// It holds a fixed event list and records whether ClearEvents was invoked.
type mockAggregate struct {
	events      []event.Event
	clearCalled bool
}

func (m *mockAggregate) ClearEvents() []event.Event {
	m.clearCalled = true
	evts := m.events
	m.events = nil
	return evts
}

// mockOutboxInserter records every Insert call and can inject a per-call error.
type mockOutboxInserter struct {
	calls     []insertCall
	injectErr error
}

type insertCall struct {
	evt  event.Event
	meta event.Metadata
}

func (m *mockOutboxInserter) Insert(_ context.Context, evt event.Event, meta event.Metadata) error {
	m.calls = append(m.calls, insertCall{evt: evt, meta: meta})
	return m.injectErr
}

// stubEvent is a minimal event.Event implementation used in tests.
type stubEvent struct {
	id        string
	eventType string
}

func newStubEvent(id, eventType string) *stubEvent { return &stubEvent{id: id, eventType: eventType} }

func (e *stubEvent) EventID() string       { return e.id }
func (e *stubEvent) EventType() string     { return e.eventType }
func (e *stubEvent) AggregateID() string   { return "agg-1" }
func (e *stubEvent) AggregateType() string { return "TestAggregate" }
func (e *stubEvent) OccurredAt() time.Time { return time.Now().UTC() }
func (e *stubEvent) Version() int          { return 1 }
func (e *stubEvent) Payload() any          { return nil }
func (e *stubEvent) GetMetadata() event.Metadata {
	return event.Metadata{}
}

// ---------------------------------------------------------------------------
// Execute — happy path
// ---------------------------------------------------------------------------

func TestExecute_HappyPath(t *testing.T) {
	t.Parallel()

	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{}
	agg := &mockAggregate{
		events: []event.Event{newStubEvent("evt-1", "order.created")},
	}
	fnCalled := false

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.Execute(context.Background(), agg, func(_ context.Context) error {
		fnCalled = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, txRunner.called, "RunInTx must be invoked")
	assert.True(t, fnCalled, "the persistence fn must be called")
	assert.True(t, agg.clearCalled, "ClearEvents must be called to drain the aggregate")
	require.Len(t, ob.calls, 1, "one event must be inserted into the outbox")
	assert.Equal(t, "order.created", ob.calls[0].evt.EventType())
	assert.Equal(t, "test-app", ob.calls[0].meta.Source, "source must be stamped on metadata")
}

// ---------------------------------------------------------------------------
// Execute — error paths
// ---------------------------------------------------------------------------

func TestExecute_FnError(t *testing.T) {
	t.Parallel()

	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{}
	agg := &mockAggregate{
		events: []event.Event{newStubEvent("evt-1", "order.created")},
	}
	persistenceErr := errors.New("db: constraint violation")

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.Execute(context.Background(), agg, func(_ context.Context) error {
		return persistenceErr
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, persistenceErr, "the original error must propagate")
	assert.False(t, agg.clearCalled, "ClearEvents must NOT be called when fn fails")
	assert.Empty(t, ob.calls, "no events must be inserted when fn fails")
}

func TestExecute_OutboxError(t *testing.T) {
	t.Parallel()

	outboxErr := errors.New("outbox: insert failed")
	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{injectErr: outboxErr}
	agg := &mockAggregate{
		events: []event.Event{newStubEvent("evt-1", "order.created")},
	}

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.Execute(context.Background(), agg, func(_ context.Context) error { return nil })

	require.Error(t, err)
	assert.ErrorIs(t, err, outboxErr, "outbox error must be wrapped and propagated")
}

func TestExecute_TxRunnerError(t *testing.T) {
	t.Parallel()

	txErr := errors.New("db: connection refused")
	txRunner := &mockTxRunner{injectErr: txErr}
	ob := &mockOutboxInserter{}
	agg := &mockAggregate{events: []event.Event{newStubEvent("evt-1", "order.created")}}
	fnCalled := false

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.Execute(context.Background(), agg, func(_ context.Context) error {
		fnCalled = true
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, txErr)
	assert.False(t, fnCalled, "fn must not be called when the tx runner fails")
	assert.False(t, agg.clearCalled, "aggregate must not be touched when the tx runner fails")
	assert.Empty(t, ob.calls)
}

// ---------------------------------------------------------------------------
// Execute — edge cases
// ---------------------------------------------------------------------------

func TestExecute_NoEvents(t *testing.T) {
	t.Parallel()

	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{}
	agg := &mockAggregate{events: nil}

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.Execute(context.Background(), agg, func(_ context.Context) error { return nil })

	require.NoError(t, err)
	assert.True(t, agg.clearCalled, "ClearEvents is still called even when there are no events")
	assert.Empty(t, ob.calls, "no outbox inserts when aggregate has no events")
}

func TestExecute_MultipleEvents(t *testing.T) {
	t.Parallel()

	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{}
	agg := &mockAggregate{
		events: []event.Event{
			newStubEvent("evt-1", "order.created"),
			newStubEvent("evt-2", "order.item.added"),
			newStubEvent("evt-3", "order.confirmed"),
		},
	}

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.Execute(context.Background(), agg, func(_ context.Context) error { return nil })

	require.NoError(t, err)
	require.Len(t, ob.calls, 3)
	assert.Equal(t, "order.created", ob.calls[0].evt.EventType())
	assert.Equal(t, "order.item.added", ob.calls[1].evt.EventType())
	assert.Equal(t, "order.confirmed", ob.calls[2].evt.EventType())
}

// ---------------------------------------------------------------------------
// ExecuteMulti tests
// ---------------------------------------------------------------------------

func TestExecuteMulti_CollectsEventsFromAllAggregates(t *testing.T) {
	t.Parallel()

	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{}
	agg1 := &mockAggregate{
		events: []event.Event{newStubEvent("evt-1", "user.registered")},
	}
	agg2 := &mockAggregate{
		events: []event.Event{
			newStubEvent("evt-2", "order.created"),
			newStubEvent("evt-3", "order.confirmed"),
		},
	}

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.ExecuteMulti(
		context.Background(),
		[]uow.Aggregate{agg1, agg2},
		func(_ context.Context) error { return nil },
	)

	require.NoError(t, err)
	assert.True(t, agg1.clearCalled, "ClearEvents must be called on agg1")
	assert.True(t, agg2.clearCalled, "ClearEvents must be called on agg2")
	require.Len(t, ob.calls, 3, "all three events must be inserted")
	assert.Equal(t, "user.registered", ob.calls[0].evt.EventType())
	assert.Equal(t, "order.created", ob.calls[1].evt.EventType())
	assert.Equal(t, "order.confirmed", ob.calls[2].evt.EventType())
}

func TestExecuteMulti_FnError(t *testing.T) {
	t.Parallel()

	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{}
	agg1 := &mockAggregate{events: []event.Event{newStubEvent("evt-1", "order.created")}}
	agg2 := &mockAggregate{events: []event.Event{newStubEvent("evt-2", "order.confirmed")}}
	persistenceErr := errors.New("save failed")

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.ExecuteMulti(
		context.Background(),
		[]uow.Aggregate{agg1, agg2},
		func(_ context.Context) error { return persistenceErr },
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, persistenceErr)
	assert.False(t, agg1.clearCalled, "agg1 must not be drained on fn failure")
	assert.False(t, agg2.clearCalled, "agg2 must not be drained on fn failure")
	assert.Empty(t, ob.calls)
}

func TestExecuteMulti_OutboxErrorAbortsRemaining(t *testing.T) {
	t.Parallel()

	outboxErr := errors.New("outbox insert failed")
	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{injectErr: outboxErr}
	agg1 := &mockAggregate{events: []event.Event{newStubEvent("evt-1", "order.created")}}
	agg2 := &mockAggregate{events: []event.Event{newStubEvent("evt-2", "order.confirmed")}}

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.ExecuteMulti(
		context.Background(),
		[]uow.Aggregate{agg1, agg2},
		func(_ context.Context) error { return nil },
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, outboxErr, "outbox error must propagate")
	assert.True(t, agg1.clearCalled, "agg1 was reached before the error")
	assert.False(t, agg2.clearCalled, "agg2 must not be touched after an outbox error")
}

func TestExecuteMulti_EmptyAggregateSlice(t *testing.T) {
	t.Parallel()

	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{}
	fnCalled := false

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.ExecuteMulti(
		context.Background(),
		[]uow.Aggregate{},
		func(_ context.Context) error {
			fnCalled = true
			return nil
		},
	)

	require.NoError(t, err)
	assert.True(t, fnCalled)
	assert.Empty(t, ob.calls)
}

// ---------------------------------------------------------------------------
// Constructor guard tests
// ---------------------------------------------------------------------------

func TestNew_NilTxRunnerPanics(t *testing.T) {
	t.Parallel()

	ob := &mockOutboxInserter{}
	assert.Panics(t, func() {
		uow.New(nil, ob, "test-app")
	})
}

func TestNew_NilOutboxStorePanics(t *testing.T) {
	t.Parallel()

	txRunner := &mockTxRunner{}
	assert.Panics(t, func() {
		uow.New(txRunner, nil, "test-app")
	})
}

func TestNew_EmptySourcePanics(t *testing.T) {
	t.Parallel()

	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{}
	assert.Panics(t, func() {
		uow.New(txRunner, ob, "")
	})
}

// ---------------------------------------------------------------------------
// ClearEvents ordering guarantee — regression test
// ---------------------------------------------------------------------------

func TestExecute_ClearEventsCalledAfterFn(t *testing.T) {
	t.Parallel()

	callOrder := make([]string, 0, 2)

	txRunner := &mockTxRunner{}
	ob := &mockOutboxInserter{}
	agg := &recordingAggregate{
		events:  []event.Event{newStubEvent("evt-1", "order.created")},
		onClear: func() { callOrder = append(callOrder, "ClearEvents") },
	}

	unit := uow.New(txRunner, ob, "test-app")

	err := unit.Execute(context.Background(), agg, func(_ context.Context) error {
		callOrder = append(callOrder, "fn")
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, []string{"fn", "ClearEvents"}, callOrder,
		"fn (aggregate save) must run before ClearEvents to prevent event loss on rollback")
}

// recordingAggregate is a test double that invokes a callback when ClearEvents
// is called, enabling call-order assertions.
type recordingAggregate struct {
	events  []event.Event
	onClear func()
}

func (r *recordingAggregate) ClearEvents() []event.Event {
	if r.onClear != nil {
		r.onClear()
	}
	evts := r.events
	r.events = nil
	return evts
}
