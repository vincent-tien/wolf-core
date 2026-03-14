package messenger

import (
	"testing"
	"time"

	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

type testMsg struct{ ID string }

func (testMsg) MessageName() string { return "test.TestMsg" }

func TestNewEnvelope_NoStamps(t *testing.T) {
	env := NewEnvelope(testMsg{ID: "1"})

	if env.Message == nil {
		t.Fatal("Message is nil")
	}
	if env.StampCount() != 0 {
		t.Errorf("StampCount() = %d, want 0", env.StampCount())
	}
	if env.Stamps() != nil {
		t.Error("Stamps() should return nil for 0 stamps")
	}
}

func TestNewEnvelope_WithStamps(t *testing.T) {
	s1 := stamp.BusNameStamp{Name: "default"}
	s2 := stamp.TraceStamp{TraceID: "t1", SpanID: "s1"}
	env := NewEnvelope(testMsg{ID: "2"}, s1, s2)

	if env.StampCount() != 2 {
		t.Errorf("StampCount() = %d, want 2", env.StampCount())
	}
}

func TestEnvelope_WithStamp_Immutability(t *testing.T) {
	original := NewEnvelope(testMsg{ID: "3"})
	added := original.WithStamp(stamp.BusNameStamp{Name: "bus1"})

	if original.StampCount() != 0 {
		t.Errorf("original modified: StampCount() = %d, want 0", original.StampCount())
	}
	if added.StampCount() != 1 {
		t.Errorf("added StampCount() = %d, want 1", added.StampCount())
	}
}

func TestEnvelope_WithoutStamp(t *testing.T) {
	env := NewEnvelope(testMsg{ID: "4"},
		stamp.BusNameStamp{Name: "a"},
		stamp.TraceStamp{TraceID: "t"},
		stamp.BusNameStamp{Name: "b"},
	)

	filtered := env.WithoutStamp(stamp.NameBusName)
	if filtered.StampCount() != 1 {
		t.Errorf("StampCount() = %d, want 1", filtered.StampCount())
	}
	if !filtered.HasStamp(stamp.NameTrace) {
		t.Error("should still have trace stamp")
	}
}

func TestEnvelope_WithoutStamp_Empty(t *testing.T) {
	env := NewEnvelope(testMsg{ID: "5"})
	same := env.WithoutStamp(stamp.NameBusName)
	if same.StampCount() != 0 {
		t.Errorf("StampCount() = %d, want 0", same.StampCount())
	}
}

func TestEnvelope_WithoutStamp_RemovesAll(t *testing.T) {
	env := NewEnvelope(testMsg{ID: "6"},
		stamp.BusNameStamp{Name: "a"},
		stamp.BusNameStamp{Name: "b"},
	)
	filtered := env.WithoutStamp(stamp.NameBusName)
	if filtered.StampCount() != 0 {
		t.Errorf("StampCount() = %d, want 0", filtered.StampCount())
	}
}

func TestEnvelope_Last(t *testing.T) {
	env := NewEnvelope(testMsg{ID: "7"},
		stamp.BusNameStamp{Name: "first"},
		stamp.TraceStamp{TraceID: "t1"},
		stamp.BusNameStamp{Name: "second"},
	)

	last := env.Last(stamp.NameBusName)
	if last == nil {
		t.Fatal("Last() returned nil")
	}
	bs, ok := last.(stamp.BusNameStamp)
	if !ok {
		t.Fatal("wrong stamp type")
	}
	if bs.Name != "second" {
		t.Errorf("Last() Name = %q, want %q", bs.Name, "second")
	}
}

func TestEnvelope_Last_NotFound(t *testing.T) {
	env := NewEnvelope(testMsg{ID: "8"})
	if env.Last(stamp.NameBusName) != nil {
		t.Error("Last() should return nil for missing stamp")
	}
}

func TestEnvelope_All(t *testing.T) {
	env := NewEnvelope(testMsg{ID: "9"},
		stamp.BusNameStamp{Name: "a"},
		stamp.TraceStamp{TraceID: "t"},
		stamp.BusNameStamp{Name: "b"},
	)

	all := env.All(stamp.NameBusName)
	if len(all) != 2 {
		t.Errorf("All() len = %d, want 2", len(all))
	}
}

func TestEnvelope_All_Empty(t *testing.T) {
	env := NewEnvelope(testMsg{ID: "10"})
	all := env.All(stamp.NameBusName)
	if len(all) != 0 {
		t.Errorf("All() len = %d, want 0", len(all))
	}
}

func TestEnvelope_HasStamp(t *testing.T) {
	env := NewEnvelope(testMsg{ID: "11"}, stamp.TraceStamp{TraceID: "t"})
	if !env.HasStamp(stamp.NameTrace) {
		t.Error("HasStamp should return true")
	}
	if env.HasStamp(stamp.NameBusName) {
		t.Error("HasStamp should return false for missing stamp")
	}
}

func TestEnvelope_CreatedAt(t *testing.T) {
	before := time.Now()
	env := NewEnvelope(testMsg{ID: "12"})
	after := time.Now()

	if env.CreatedAt().Before(before) || env.CreatedAt().After(after) {
		t.Error("CreatedAt should be between before and after")
	}
}

func TestEnvelope_MessageTypeName(t *testing.T) {
	env := NewEnvelope(testMsg{ID: "13"})
	if got := env.MessageTypeName(); got != "test.TestMsg" {
		t.Errorf("MessageTypeName() = %q, want %q", got, "test.TestMsg")
	}
}

func TestEnvelope_Stamps_ReturnsCopy(t *testing.T) {
	s1 := stamp.BusNameStamp{Name: "a"}
	env := NewEnvelope(testMsg{ID: "14"}, s1)

	stamps := env.Stamps()
	stamps[0] = stamp.TraceStamp{TraceID: "mutated"}

	// Original should be unchanged.
	original := env.Stamps()
	if _, ok := original[0].(stamp.BusNameStamp); !ok {
		t.Error("Stamps() did not return a copy — mutation leaked")
	}
}

// ── Benchmarks ──

func BenchmarkNewEnvelope_NoStamps(b *testing.B) {
	msg := testMsg{ID: "bench"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		NewEnvelope(msg)
	}
}

func BenchmarkNewEnvelope_2Stamps(b *testing.B) {
	msg := testMsg{ID: "bench"}
	s1 := stamp.BusNameStamp{Name: "default"}
	s2 := stamp.TraceStamp{TraceID: "abc", SpanID: "def"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		NewEnvelope(msg, s1, s2)
	}
}

func BenchmarkEnvelope_WithStamp(b *testing.B) {
	env := NewEnvelope(testMsg{ID: "bench"})
	s := stamp.BusNameStamp{Name: "default"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		env.WithStamp(s)
	}
}

func BenchmarkEnvelope_Last(b *testing.B) {
	env := NewEnvelope(testMsg{ID: "bench"},
		stamp.BusNameStamp{Name: "a"},
		stamp.TraceStamp{TraceID: "t"},
		stamp.BusNameStamp{Name: "b"},
		stamp.DelayStamp{Duration: time.Second},
		stamp.BusNameStamp{Name: "c"},
	)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		env.Last(stamp.NameBusName)
	}
}
