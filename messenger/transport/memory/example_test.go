package memory_test

import (
	"context"
	"fmt"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/transport/memory"
)

type exMsg struct{ ID string }

func (exMsg) MessageName() string { return "example.Msg" }

func ExampleTransport() {
	mem := memory.New(memory.WithBufferSize(10))
	defer mem.Close()

	// Send an envelope.
	env := messenger.NewEnvelope(exMsg{ID: "1"})
	_ = mem.Send(context.Background(), env)

	// Receive it.
	envelopes, _ := mem.Get(context.Background())
	fmt.Printf("received: %d\n", len(envelopes))

	// Ack to remove from pending.
	_ = mem.Ack(context.Background(), envelopes[0])
	fmt.Printf("pending after ack: %d\n", mem.Len())

	// Output:
	// received: 1
	// pending after ack: 0
}
