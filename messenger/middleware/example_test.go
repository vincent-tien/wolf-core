package middleware_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/middleware"
)

type exCmd struct{ ID string }

func (exCmd) MessageName() string { return "example.Cmd" }

func passNext(_ context.Context, env messenger.Envelope) (messenger.DispatchResult, error) {
	return messenger.DispatchResult{Envelope: env}, nil
}

func ExampleNewRecovery() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mw := middleware.NewRecovery(logger)
	env := messenger.NewEnvelope(exCmd{ID: "1"})

	// Recovery converts panics to errors.
	_, err := mw.Handle(context.Background(), env, func(_ context.Context, _ messenger.Envelope) (messenger.DispatchResult, error) {
		panic("something went wrong")
	})
	fmt.Printf("recovered: %v\n", err != nil)

	// Output:
	// recovered: true
}

func ExampleNewTimeout() {
	mw := middleware.NewTimeout(50 * time.Millisecond)
	env := messenger.NewEnvelope(exCmd{ID: "2"})

	// Fast handler succeeds.
	_, err := mw.Handle(context.Background(), env, passNext)
	fmt.Printf("fast: err=%v\n", err)

	// Output:
	// fast: err=<nil>
}

func ExampleNewValidation() {
	mw := middleware.NewValidation(func(msg any) error {
		cmd := msg.(exCmd)
		if cmd.ID == "" {
			return fmt.Errorf("ID required")
		}
		return nil
	})

	// Valid message passes through.
	env := messenger.NewEnvelope(exCmd{ID: "3"})
	_, err := mw.Handle(context.Background(), env, passNext)
	fmt.Printf("valid: err=%v\n", err)

	// Invalid message short-circuits.
	env2 := messenger.NewEnvelope(exCmd{ID: ""})
	_, err2 := mw.Handle(context.Background(), env2, passNext)
	fmt.Printf("invalid: err=%v\n", err2)

	// Output:
	// valid: err=<nil>
	// invalid: err=messenger: validation failed for example.Cmd: ID required
}
