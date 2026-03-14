package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
	"github.com/vincent-tien/wolf-core/messenger/transport/memory"
)

type testCmd struct{ ID string }

func (testCmd) MessageName() string { return "test.Cmd" }

func TestSendAndReceive(t *testing.T) {
	tr := memory.New(memory.WithBufferSize(10))
	defer tr.Close()

	env := messenger.NewEnvelope(testCmd{ID: "1"})
	if err := tr.Send(context.Background(), env); err != nil {
		t.Fatalf("Send: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := tr.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Get returned %d envelopes, want 1", len(got))
	}
	msg, ok := got[0].Message.(testCmd)
	if !ok {
		t.Fatalf("message type = %T, want testCmd", got[0].Message)
	}
	if msg.ID != "1" {
		t.Errorf("msg.ID = %q, want %q", msg.ID, "1")
	}
}

func TestAckRemovesMessage(t *testing.T) {
	tr := memory.New(memory.WithBufferSize(10))
	defer tr.Close()

	env := messenger.NewEnvelope(testCmd{ID: "2"})
	_ = tr.Send(context.Background(), env)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, _ := tr.Get(ctx)
	if err := tr.Ack(context.Background(), got[0]); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	// After ack, buffer should be empty.
	if tr.Len() != 0 {
		t.Errorf("Len() = %d after ack, want 0", tr.Len())
	}
}

func TestRejectRequeues(t *testing.T) {
	tr := memory.New(memory.WithBufferSize(10))
	defer tr.Close()

	env := messenger.NewEnvelope(testCmd{ID: "3"})
	_ = tr.Send(context.Background(), env)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, _ := tr.Get(ctx)
	if err := tr.Reject(context.Background(), got[0], nil); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	// Should be able to Get again after reject.
	got2, err := tr.Get(ctx)
	if err != nil {
		t.Fatalf("Get after reject: %v", err)
	}
	if len(got2) != 1 {
		t.Fatalf("expected 1 message after reject, got %d", len(got2))
	}
}

func TestBufferedSendReceive(t *testing.T) {
	const n = 5
	tr := memory.New(memory.WithBufferSize(n))
	defer tr.Close()

	for i := range n {
		env := messenger.NewEnvelope(testCmd{ID: string(rune('A' + i))})
		if err := tr.Send(context.Background(), env); err != nil {
			t.Fatalf("Send[%d]: %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for range n {
		got, err := tr.Get(ctx)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("Get returned %d, want 1", len(got))
		}
		_ = tr.Ack(context.Background(), got[0])
	}
}

func TestContextCancellation(t *testing.T) {
	tr := memory.New(memory.WithBufferSize(10))
	defer tr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := tr.Get(ctx)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestClosePreventsSend(t *testing.T) {
	tr := memory.New(memory.WithBufferSize(10))
	_ = tr.Close()

	err := tr.Send(context.Background(), messenger.NewEnvelope(testCmd{ID: "x"}))
	if err == nil {
		t.Error("expected error sending to closed transport")
	}
}

func TestStampsPreserved(t *testing.T) {
	tr := memory.New(memory.WithBufferSize(10))
	defer tr.Close()

	env := messenger.NewEnvelope(testCmd{ID: "4"},
		stamp.TraceStamp{TraceID: "trace-1", SpanID: "span-1"},
		stamp.BusNameStamp{Name: "default"},
	)
	_ = tr.Send(context.Background(), env)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, _ := tr.Get(ctx)
	// 2 user stamps + 1 internal memory.internal_id stamp = 3
	if got[0].StampCount() != 3 {
		t.Errorf("StampCount() = %d, want 3", got[0].StampCount())
	}
	if !got[0].HasStamp(stamp.NameTrace) {
		t.Error("trace stamp not preserved")
	}
}

func TestConcurrentSendGet(t *testing.T) {
	tr := memory.New(memory.WithBufferSize(100))
	defer tr.Close()

	const n = 50
	done := make(chan struct{})

	// Sender goroutine.
	go func() {
		defer close(done)
		for i := range n {
			env := messenger.NewEnvelope(testCmd{ID: string(rune('0' + i%10))})
			if err := tr.Send(context.Background(), env); err != nil {
				return
			}
		}
	}()

	// Receiver goroutine.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	received := 0
	for received < n {
		got, err := tr.Get(ctx)
		if err != nil {
			break
		}
		for _, env := range got {
			_ = tr.Ack(context.Background(), env)
			received++
		}
	}

	<-done

	if received != n {
		t.Errorf("received %d messages, want %d", received, n)
	}
}

func TestCloseIdempotent(t *testing.T) {
	tr := memory.New(memory.WithBufferSize(10))
	if err := tr.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestName(t *testing.T) {
	tr := memory.New(memory.WithName("test-transport"))
	defer tr.Close()
	if got := tr.Name(); got != "test-transport" {
		t.Errorf("Name() = %q, want %q", got, "test-transport")
	}
}

func TestFactory(t *testing.T) {
	f := memory.Factory{}
	if !f.Supports("memory://") {
		t.Error("Factory should support memory:// DSN")
	}
	if f.Supports("nats://localhost") {
		t.Error("Factory should not support nats:// DSN")
	}

	tr, err := f.Create("memory://", map[string]any{"buffer_size": 50})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer tr.Close()
	if tr.Name() != "memory" {
		t.Errorf("Name() = %q, want %q", tr.Name(), "memory")
	}
}
