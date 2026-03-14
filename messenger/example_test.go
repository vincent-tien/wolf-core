package messenger_test

import (
	"context"
	"fmt"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

// orderCmd is a command for examples.
type orderCmd struct {
	OrderID string
}

func (orderCmd) MessageName() string { return "order.PlaceOrder" }

// orderQuery is a query for examples.
type orderQuery struct {
	OrderID string
}

func (orderQuery) MessageName() string { return "order.GetOrder" }

// orderResult is a query result for examples.
type orderResult struct {
	OrderID string
	Status  string
}

func ExampleMessageBus_Dispatch() {
	bus := messenger.NewBus("example")

	// Register a command handler.
	messenger.RegisterCommandFunc(bus.Handlers(), func(ctx context.Context, cmd orderCmd) error {
		fmt.Printf("handling order %s\n", cmd.OrderID)
		return nil
	})

	// Dispatch synchronously (default).
	result, err := bus.Dispatch(context.Background(), orderCmd{OrderID: "ord-1"})
	if err != nil {
		panic(err)
	}
	fmt.Printf("async: %v\n", result.Async)

	// Output:
	// handling order ord-1
	// async: false
}

func ExampleMessageBus_Query() {
	bus := messenger.NewBus("example")

	// Register a query handler that returns a result.
	messenger.RegisterQueryFunc(bus.Handlers(), func(ctx context.Context, q orderQuery) (orderResult, error) {
		return orderResult{OrderID: q.OrderID, Status: "confirmed"}, nil
	})

	// Query always executes synchronously.
	raw, err := bus.Query(context.Background(), orderQuery{OrderID: "ord-1"})
	if err != nil {
		panic(err)
	}
	order := raw.(orderResult)
	fmt.Printf("order %s: %s\n", order.OrderID, order.Status)

	// Output:
	// order ord-1: confirmed
}

func ExampleMessageBus_Dispatch_forceSync() {
	bus := messenger.NewBus("example")

	messenger.RegisterCommandFunc(bus.Handlers(), func(ctx context.Context, cmd orderCmd) error {
		fmt.Printf("forced sync: %s\n", cmd.OrderID)
		return nil
	})

	// ForceSyncStamp overrides router — always executes sync.
	result, err := bus.Dispatch(context.Background(), orderCmd{OrderID: "ord-2"}, stamp.ForceSyncStamp{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("async: %v\n", result.Async)

	// Output:
	// forced sync: ord-2
	// async: false
}
