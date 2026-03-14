package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithCorrelationID(t *testing.T) {
	evt := NewEvent("test.v1", nil, WithCorrelationID("corr-123"))
	assert.Equal(t, "corr-123", evt.GetMetadata().CorrelationID)
}

func TestWithCausationID(t *testing.T) {
	evt := NewEvent("test.v1", nil, WithCausationID("cause-456"))
	assert.Equal(t, "cause-456", evt.GetMetadata().CausationID)
}

func TestWithTraceID(t *testing.T) {
	evt := NewEvent("test.v1", nil, WithTraceID("trace-789"))
	assert.Equal(t, "trace-789", evt.GetMetadata().TraceID)
}

func TestWithSource(t *testing.T) {
	evt := NewEvent("test.v1", nil, WithSource("order-service"))
	assert.Equal(t, "order-service", evt.GetMetadata().Source)
}

func TestWithVersion(t *testing.T) {
	evt := NewEvent("test.v1", nil, WithVersion(3))
	assert.Equal(t, 3, evt.Version())
}

func TestWithAggregateInfo(t *testing.T) {
	evt := NewEvent("test.v1", nil, WithAggregateInfo("agg-1", "Order"))
	assert.Equal(t, "agg-1", evt.AggregateID())
	assert.Equal(t, "Order", evt.AggregateType())
}

func TestWithMetadata(t *testing.T) {
	meta := Metadata{
		TraceID:       "t1",
		CorrelationID: "c1",
		CausationID:   "cs1",
		Source:        "svc",
	}
	evt := NewEvent("test.v1", nil, WithMetadata(meta))
	assert.Equal(t, meta, evt.GetMetadata())
}

func TestMultipleOptions_Applied_InOrder(t *testing.T) {
	evt := NewEvent("test.v1", nil,
		WithTraceID("trace-1"),
		WithCorrelationID("corr-1"),
		WithAggregateInfo("agg-1", "Test"),
		WithSource("svc"),
		WithVersion(2),
	)

	assert.Equal(t, "trace-1", evt.GetMetadata().TraceID)
	assert.Equal(t, "corr-1", evt.GetMetadata().CorrelationID)
	assert.Equal(t, "agg-1", evt.AggregateID())
	assert.Equal(t, "Test", evt.AggregateType())
	assert.Equal(t, "svc", evt.GetMetadata().Source)
	assert.Equal(t, 2, evt.Version())
}

func TestEventOptionFunc_IsAnEventOption(t *testing.T) {
	// Verify the type satisfies the interface at compile time.
	var _ EventOption = EventOptionFunc(func(_ *baseEvent) {})
}
