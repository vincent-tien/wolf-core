package messenger

import (
	"testing"
)

// typerMsg implements Typer interface.
type typerMsg struct{}

func (typerMsg) MessageName() string { return "test.TyperMsg" }

// plainMsg does NOT implement Typer — uses reflect fallback.
type plainMsg struct {
	Value int
}

func TestTypeNameOf_Typer(t *testing.T) {
	got := TypeNameOf(typerMsg{})
	if got != "test.TyperMsg" {
		t.Errorf("TypeNameOf(typerMsg) = %q, want %q", got, "test.TyperMsg")
	}
}

func TestTypeNameOf_CachedFallback(t *testing.T) {
	// First call: reflect + cache
	name1 := TypeNameOf(plainMsg{Value: 1})
	// Second call: cached
	name2 := TypeNameOf(plainMsg{Value: 2})

	if name1 != name2 {
		t.Errorf("TypeNameOf not deterministic: %q vs %q", name1, name2)
	}
	if name1 == "" {
		t.Error("TypeNameOf returned empty string")
	}
}

func TestTypeNameOf_ReflectLastResort(t *testing.T) {
	// Use a local type that has never been seen before.
	type neverSeen struct{ X int }
	got := TypeNameOf(neverSeen{X: 42})
	if got == "" {
		t.Error("TypeNameOf returned empty string for unseen type")
	}
}

func TestPreregisterType_AvoidReflect(t *testing.T) {
	type preregistered struct{ Y int }
	PreregisterType[preregistered]()

	// After pre-registration, should be in cache.
	name1 := TypeNameOf(preregistered{Y: 1})
	name2 := TypeNameOf(preregistered{Y: 2})
	if name1 != name2 {
		t.Errorf("pre-registered type not deterministic: %q vs %q", name1, name2)
	}
}

func TestTypeNameOf_Deterministic(t *testing.T) {
	// Same type always returns same string.
	for i := 0; i < 100; i++ {
		got := TypeNameOf(typerMsg{})
		if got != "test.TyperMsg" {
			t.Fatalf("iteration %d: got %q", i, got)
		}
	}
}

// ── Benchmarks ──

func BenchmarkTypeNameOf_Typer(b *testing.B) {
	msg := typerMsg{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		TypeNameOf(msg)
	}
}

func BenchmarkTypeNameOf_Cached(b *testing.B) {
	msg := plainMsg{Value: 42}
	// Warm up cache.
	TypeNameOf(msg)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		TypeNameOf(msg)
	}
}

func BenchmarkTypeNameOf_Preregistered(b *testing.B) {
	type benchPreReg struct{ V int }
	PreregisterType[benchPreReg]()
	msg := benchPreReg{V: 1}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		TypeNameOf(msg)
	}
}
