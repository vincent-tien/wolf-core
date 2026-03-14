package worker_test

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/transport"
	"github.com/vincent-tien/wolf-core/messenger/transport/memory"
	"github.com/vincent-tien/wolf-core/messenger/worker"
)

type workerCmd struct{ ID string }

func (workerCmd) MessageName() string { return "worker.TestCmd" }

var testLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

func newTestBus() *messenger.MessageBus {
	bus := messenger.NewBus("test")
	return bus
}

func TestWorker_ConsumesAndDispatches(t *testing.T) {
	bus := newTestBus()
	received := make(chan string, 1)
	messenger.RegisterCommandFunc[workerCmd](bus.Handlers(), func(_ context.Context, cmd workerCmd) error {
		received <- cmd.ID
		return nil
	})

	mem := memory.New(memory.WithBufferSize(10))
	defer mem.Close()

	env := messenger.NewEnvelope(workerCmd{ID: "ord-1"})
	_ = mem.Send(context.Background(), env)

	receivers := map[string]transport.Receiver{"memory": mem}
	w := worker.New(bus, receivers,
		worker.WithLogger(testLogger),
		worker.WithPollInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go w.Run(ctx, "memory")

	select {
	case id := <-received:
		if id != "ord-1" {
			t.Errorf("received ID = %q, want %q", id, "ord-1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestWorker_AcksOnSuccess(t *testing.T) {
	bus := newTestBus()
	messenger.RegisterCommandFunc[workerCmd](bus.Handlers(), func(_ context.Context, _ workerCmd) error {
		return nil
	})

	mem := memory.New(memory.WithBufferSize(10))
	defer mem.Close()

	_ = mem.Send(context.Background(), messenger.NewEnvelope(workerCmd{ID: "ack-test"}))

	receivers := map[string]transport.Receiver{"memory": mem}
	w := worker.New(bus, receivers,
		worker.WithLogger(testLogger),
		worker.WithPollInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go w.Run(ctx, "memory")

	// Give worker time to process.
	time.Sleep(200 * time.Millisecond)
	cancel()

	// Buffer should be empty after ack.
	if mem.Len() != 0 {
		t.Errorf("buffer should be empty after ack, got %d", mem.Len())
	}
}

func TestWorker_RejectsOnError(t *testing.T) {
	bus := newTestBus()
	var calls atomic.Int32
	successCh := make(chan struct{})
	messenger.RegisterCommandFunc[workerCmd](bus.Handlers(), func(_ context.Context, _ workerCmd) error {
		n := calls.Add(1)
		if n == 1 {
			return errTest
		}
		close(successCh)
		return nil
	})

	mem := memory.New(memory.WithBufferSize(10))

	_ = mem.Send(context.Background(), messenger.NewEnvelope(workerCmd{ID: "reject-test"}))

	receivers := map[string]transport.Receiver{"memory": mem}
	w := worker.New(bus, receivers,
		worker.WithLogger(testLogger),
		worker.WithPollInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())

	go w.Run(ctx, "memory")

	// Wait for successful redelivery.
	select {
	case <-successCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for redelivery")
	}

	cancel()
	// Give worker time to exit before closing transport.
	time.Sleep(100 * time.Millisecond)
	mem.Close()

	if got := calls.Load(); got < 2 {
		t.Errorf("expected at least 2 calls (initial + redelivery), got %d", got)
	}
}

func TestWorker_GracefulShutdown(t *testing.T) {
	bus := newTestBus()
	processing := make(chan struct{})
	done := make(chan struct{})

	messenger.RegisterCommandFunc[workerCmd](bus.Handlers(), func(_ context.Context, _ workerCmd) error {
		close(processing)
		time.Sleep(200 * time.Millisecond) // simulate work
		close(done)
		return nil
	})

	mem := memory.New(memory.WithBufferSize(10))
	defer mem.Close()

	_ = mem.Send(context.Background(), messenger.NewEnvelope(workerCmd{ID: "graceful"}))

	receivers := map[string]transport.Receiver{"memory": mem}
	w := worker.New(bus, receivers,
		worker.WithLogger(testLogger),
		worker.WithPollInterval(10*time.Millisecond),
		worker.WithShutdownTimeout(2*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx, "memory")

	// Wait for handler to start processing.
	<-processing
	// Cancel while handler is working.
	cancel()
	// Handler should complete before shutdown.
	select {
	case <-done:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not complete during graceful shutdown")
	}
}

func TestWorker_UnknownTransport(t *testing.T) {
	bus := newTestBus()
	w := worker.New(bus, nil, worker.WithLogger(testLogger))

	err := w.Run(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for unknown transport")
	}
}

var errTest = errSentinel("test error")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
