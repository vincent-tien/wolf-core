package event

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEvent_FieldsArePopulated(t *testing.T) {
	before := time.Now().UTC().Add(-time.Millisecond)
	type payload struct{ Name string }

	evt := NewEvent("order.created.v1", &payload{Name: "test"},
		WithAggregateInfo("order-123", "Order"),
	)

	assert.NotEmpty(t, evt.EventID(), "ID must be set")
	assert.Equal(t, "order.created.v1", evt.EventType())
	assert.Equal(t, "order-123", evt.AggregateID())
	assert.Equal(t, "Order", evt.AggregateType())
	assert.Equal(t, 1, evt.Version(), "schema version must default to 1")
	assert.True(t, evt.OccurredAt().After(before), "OccurredAt must be recent")
	assert.True(t, evt.OccurredAt().Before(time.Now().UTC().Add(time.Millisecond)), "OccurredAt must not be in the future")
	assert.NotNil(t, evt.Payload(), "Payload must be set")
}

func TestNewEvent_IDIsValidUUID(t *testing.T) {
	evt := NewEvent("order.created.v1", nil, WithAggregateInfo("order-123", "Order"))

	id := evt.EventID()
	require.NotEmpty(t, id)
	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	parts := strings.Split(id, "-")
	assert.Len(t, parts, 5, "UUID must have 5 hyphen-separated parts")
}

func TestNewEvent_UniqueIDsPerCall(t *testing.T) {
	a := NewEvent("x.y.v1", nil)
	b := NewEvent("x.y.v1", nil)
	assert.NotEqual(t, a.EventID(), b.EventID(), "each call must produce a unique ID")
}

func TestNewEvent_InterfaceCompliance(t *testing.T) {
	evt := NewEvent("payment.processed.v1", nil,
		WithAggregateInfo("pay-456", "Payment"),
	)

	var _ Event = evt

	assert.Equal(t, "payment.processed.v1", evt.EventType())
	assert.Equal(t, "pay-456", evt.AggregateID())
	assert.Equal(t, "Payment", evt.AggregateType())
	assert.Equal(t, 1, evt.Version())
}

func TestNewEvent_MetadataDefaultsToZeroValue(t *testing.T) {
	evt := NewEvent("a.b.v1", nil)
	meta := evt.GetMetadata()
	assert.Empty(t, meta.TraceID)
	assert.Empty(t, meta.CorrelationID)
	assert.Empty(t, meta.CausationID)
	assert.Empty(t, meta.Source)
}

func TestNewEvent_MetadataCanBeSetViaOptions(t *testing.T) {
	evt := NewEvent("a.b.v1", nil,
		WithTraceID("trace-1"),
		WithCorrelationID("corr-1"),
		WithCausationID("cause-1"),
		WithSource("order-service"),
	)

	meta := evt.GetMetadata()
	assert.Equal(t, "trace-1", meta.TraceID)
	assert.Equal(t, "corr-1", meta.CorrelationID)
	assert.Equal(t, "cause-1", meta.CausationID)
	assert.Equal(t, "order-service", meta.Source)
}

func TestNewEvent_WithVersionOverridesDefault(t *testing.T) {
	evt := NewEvent("a.b.v2", nil, WithVersion(2))
	assert.Equal(t, 2, evt.Version())
}

func TestNewEvent_WithMetadataSetsAll(t *testing.T) {
	meta := Metadata{
		TraceID:       "t1",
		CorrelationID: "c1",
		CausationID:   "cs1",
		Source:        "svc",
	}
	evt := NewEvent("x.y.v1", nil, WithMetadata(meta))
	assert.Equal(t, meta, evt.GetMetadata())
}

func TestNewEvent_PayloadIsAccessible(t *testing.T) {
	type myPayload struct {
		SKU  string
		Name string
	}
	p := &myPayload{SKU: "ABC-123", Name: "Widget"}
	evt := NewEvent("product.created.v1", p)

	got, ok := evt.Payload().(*myPayload)
	require.True(t, ok)
	assert.Equal(t, "ABC-123", got.SKU)
	assert.Equal(t, "Widget", got.Name)
}

func TestNewEvent_NilPayload(t *testing.T) {
	evt := NewEvent("product.activated.v1", nil)
	assert.Nil(t, evt.Payload())
}

func TestNewEvent_AllEventTypes(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		aggID     string
		aggType   string
	}{
		{
			name:      "order created",
			eventType: "order.created.v1",
			aggID:     "order-1",
			aggType:   "Order",
		},
		{
			name:      "payment processed",
			eventType: "payment.processed.v1",
			aggID:     "pay-2",
			aggType:   "Payment",
		},
		{
			name:      "user registered",
			eventType: "iam.user.registered.v1",
			aggID:     "user-3",
			aggType:   "User",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evt := NewEvent(tc.eventType, nil,
				WithAggregateInfo(tc.aggID, tc.aggType),
			)

			assert.Equal(t, tc.eventType, evt.EventType())
			assert.Equal(t, tc.aggID, evt.AggregateID())
			assert.Equal(t, tc.aggType, evt.AggregateType())
			assert.NotEmpty(t, evt.EventID())
			assert.Equal(t, 1, evt.Version())
		})
	}
}

func TestMetadata_ToMap(t *testing.T) {
	meta := Metadata{
		TraceID:       "t1",
		CorrelationID: "c1",
		CausationID:   "cs1",
		Source:        "svc",
	}
	m := meta.ToMap()
	assert.Equal(t, "t1", m["trace_id"])
	assert.Equal(t, "c1", m["correlation_id"])
	assert.Equal(t, "cs1", m["causation_id"])
	assert.Equal(t, "svc", m["source"])
}

func TestMetadata_ToMap_EmptyFieldsOmitted(t *testing.T) {
	meta := Metadata{TraceID: "t1"}
	m := meta.ToMap()
	assert.Len(t, m, 1)
	assert.Equal(t, "t1", m["trace_id"])
}

func TestMetadata_ToMap_AllEmpty_ReturnsNil(t *testing.T) {
	meta := Metadata{}
	m := meta.ToMap()
	assert.Nil(t, m, "ToMap must return nil when all fields are empty to avoid allocation")
}

func TestMetadataFromMap(t *testing.T) {
	m := map[string]string{
		"trace_id":       "t1",
		"correlation_id": "c1",
		"causation_id":   "cs1",
		"source":         "svc",
	}
	meta := MetadataFromMap(m)
	assert.Equal(t, "t1", meta.TraceID)
	assert.Equal(t, "c1", meta.CorrelationID)
	assert.Equal(t, "cs1", meta.CausationID)
	assert.Equal(t, "svc", meta.Source)
}
