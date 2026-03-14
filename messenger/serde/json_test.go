package serde_test

import (
	"encoding/json"
	"testing"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/serde"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

type orderCmd struct {
	OrderID string `json:"order_id"`
	Amount  int    `json:"amount"`
}

func (orderCmd) MessageName() string  { return "order.CreateOrderCmd" }
func (orderCmd) MessageVersion() int { return 1 }

type orderCmdV2 struct {
	OrderID  string `json:"order_id"`
	Amount   int    `json:"amount"`
	Currency string `json:"currency"`
}

func (orderCmdV2) MessageName() string  { return "order.CreateOrderCmd" }
func (orderCmdV2) MessageVersion() int { return 2 }

func newTestSerializer() (*serde.JSONSerializer, *serde.TypeRegistry) {
	reg := serde.NewTypeRegistry()
	serde.RegisterMessage[orderCmd](reg, "order.CreateOrderCmd", 1)
	return serde.NewJSONSerializer(reg, "test-service"), reg
}

func TestJSON_RoundTrip_PreservesFields(t *testing.T) {
	ser, _ := newTestSerializer()

	original := messenger.NewEnvelope(orderCmd{OrderID: "ord-1", Amount: 100})
	data, err := ser.Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := ser.Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	msg, ok := decoded.Message.(orderCmd)
	if !ok {
		t.Fatalf("message type = %T, want orderCmd", decoded.Message)
	}
	if msg.OrderID != "ord-1" {
		t.Errorf("OrderID = %q, want %q", msg.OrderID, "ord-1")
	}
	if msg.Amount != 100 {
		t.Errorf("Amount = %d, want %d", msg.Amount, 100)
	}
}

func TestJSON_RoundTrip_PreservesStamps(t *testing.T) {
	ser, _ := newTestSerializer()

	original := messenger.NewEnvelope(orderCmd{OrderID: "ord-2", Amount: 50},
		stamp.TraceStamp{TraceID: "trace-1", SpanID: "span-1"},
		stamp.BusNameStamp{Name: "default"},
	)

	data, err := ser.Encode(original)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, err := ser.Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if decoded.StampCount() != 2 {
		t.Errorf("StampCount = %d, want 2", decoded.StampCount())
	}
	if !decoded.HasStamp(stamp.NameTrace) {
		t.Error("trace stamp not preserved")
	}
	if !decoded.HasStamp(stamp.NameBusName) {
		t.Error("bus_name stamp not preserved")
	}

	ts := decoded.Last(stamp.NameTrace).(stamp.TraceStamp)
	if ts.TraceID != "trace-1" {
		t.Errorf("TraceID = %q, want %q", ts.TraceID, "trace-1")
	}
}

func TestJSON_Decode_UnknownFields_NoError(t *testing.T) {
	ser, _ := newTestSerializer()

	// Simulate wire payload with extra fields.
	wire := `{
		"schema_version": 1,
		"message_type": "order.CreateOrderCmd",
		"message_version": 1,
		"payload": {"order_id": "ord-3", "amount": 75, "extra_field": "ignored"},
		"stamps": [],
		"id": "test-id",
		"source": "other-service",
		"created_at": "2025-01-01T00:00:00Z"
	}`

	decoded, err := ser.Decode([]byte(wire))
	if err != nil {
		t.Fatalf("Decode with unknown fields: %v", err)
	}
	msg := decoded.Message.(orderCmd)
	if msg.OrderID != "ord-3" {
		t.Errorf("OrderID = %q, want %q", msg.OrderID, "ord-3")
	}
}

func TestJSON_Decode_MissingFields_ZeroValues(t *testing.T) {
	ser, _ := newTestSerializer()

	wire := `{
		"schema_version": 1,
		"message_type": "order.CreateOrderCmd",
		"message_version": 1,
		"payload": {"order_id": "ord-4"},
		"stamps": [],
		"id": "test-id",
		"source": "other-service",
		"created_at": "2025-01-01T00:00:00Z"
	}`

	decoded, err := ser.Decode([]byte(wire))
	if err != nil {
		t.Fatalf("Decode with missing fields: %v", err)
	}
	msg := decoded.Message.(orderCmd)
	if msg.Amount != 0 {
		t.Errorf("Amount = %d, want 0 (zero value)", msg.Amount)
	}
}

func TestJSON_Upcaster_V1ToV2(t *testing.T) {
	reg := serde.NewTypeRegistry()
	serde.RegisterMessage[orderCmd](reg, "order.CreateOrderCmd", 1)
	serde.RegisterMessage[orderCmdV2](reg, "order.CreateOrderCmd", 2)

	// Upcaster: add default currency.
	reg.RegisterUpcaster("order.CreateOrderCmd", 1, func(old json.RawMessage) (json.RawMessage, error) {
		var m map[string]any
		if err := json.Unmarshal(old, &m); err != nil {
			return nil, err
		}
		m["currency"] = "USD"
		return json.Marshal(m)
	})

	ser := serde.NewJSONSerializer(reg, "test-service")

	// V1 wire message.
	wire := `{
		"schema_version": 1,
		"message_type": "order.CreateOrderCmd",
		"message_version": 1,
		"payload": {"order_id": "ord-5", "amount": 200},
		"stamps": [],
		"id": "test-id",
		"source": "old-service",
		"created_at": "2025-01-01T00:00:00Z"
	}`

	decoded, err := ser.Decode([]byte(wire))
	if err != nil {
		t.Fatalf("Decode v1 with upcaster: %v", err)
	}

	msg, ok := decoded.Message.(orderCmdV2)
	if !ok {
		t.Fatalf("message type = %T, want orderCmdV2", decoded.Message)
	}
	if msg.Currency != "USD" {
		t.Errorf("Currency = %q, want %q", msg.Currency, "USD")
	}
	if msg.OrderID != "ord-5" {
		t.Errorf("OrderID = %q, want %q", msg.OrderID, "ord-5")
	}
}

func TestJSON_Decode_UnknownMessageType_Error(t *testing.T) {
	ser, _ := newTestSerializer()

	wire := `{
		"schema_version": 1,
		"message_type": "unknown.MsgType",
		"message_version": 1,
		"payload": {},
		"stamps": [],
		"id": "test-id",
		"source": "other-service",
		"created_at": "2025-01-01T00:00:00Z"
	}`

	_, err := ser.Decode([]byte(wire))
	if err == nil {
		t.Error("expected error for unknown message type")
	}
}

func TestJSON_Decode_UnknownStamps_Skipped(t *testing.T) {
	ser, _ := newTestSerializer()

	wire := `{
		"schema_version": 1,
		"message_type": "order.CreateOrderCmd",
		"message_version": 1,
		"payload": {"order_id": "ord-6", "amount": 10},
		"stamps": [
			{"name": "messenger.trace", "value": {"TraceID": "t1", "SpanID": "s1"}},
			{"name": "custom.unknown_stamp", "value": {"foo": "bar"}}
		],
		"id": "test-id",
		"source": "other-service",
		"created_at": "2025-01-01T00:00:00Z"
	}`

	decoded, err := ser.Decode([]byte(wire))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	// Unknown stamp should be skipped, only trace preserved.
	if decoded.StampCount() != 1 {
		t.Errorf("StampCount = %d, want 1 (unknown stamp skipped)", decoded.StampCount())
	}
}

// ── Benchmarks ──

func BenchmarkJSONEncode(b *testing.B) {
	ser, _ := newTestSerializer()
	env := messenger.NewEnvelope(orderCmd{OrderID: "bench", Amount: 42},
		stamp.TraceStamp{TraceID: "t", SpanID: "s"},
	)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ser.Encode(env)
	}
}

func BenchmarkJSONDecode(b *testing.B) {
	ser, _ := newTestSerializer()
	env := messenger.NewEnvelope(orderCmd{OrderID: "bench", Amount: 42},
		stamp.TraceStamp{TraceID: "t", SpanID: "s"},
	)
	data, _ := ser.Encode(env)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ser.Decode(data)
	}
}
