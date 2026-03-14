package cqrs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/cqrs"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

type testCommand struct{ value string }
type testQuery struct{}

// ---------------------------------------------------------------------------
// CommandHandlerFunc
// ---------------------------------------------------------------------------

func TestCommandHandlerFunc(t *testing.T) {
	t.Parallel()

	t.Run("delegates to underlying function on success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		want := "result"
		fn := cqrs.CommandHandlerFunc[testCommand, string](func(_ context.Context, cmd testCommand) (string, error) {
			return cmd.value, nil
		})

		// Act
		got, err := fn.Handle(context.Background(), testCommand{value: want})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("propagates error returned by underlying function", func(t *testing.T) {
		t.Parallel()

		// Arrange
		sentinel := errors.New("command error")
		fn := cqrs.CommandHandlerFunc[testCommand, string](func(_ context.Context, _ testCommand) (string, error) {
			return "", sentinel
		})

		// Act
		_, err := fn.Handle(context.Background(), testCommand{})

		// Assert
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("passes context and command to underlying function", func(t *testing.T) {
		t.Parallel()

		// Arrange
		type ctxKey string
		ctx := context.WithValue(context.Background(), ctxKey("k"), "v")
		var capturedCtx context.Context
		var capturedCmd testCommand

		fn := cqrs.CommandHandlerFunc[testCommand, string](func(c context.Context, cmd testCommand) (string, error) {
			capturedCtx = c
			capturedCmd = cmd
			return "", nil
		})

		// Act
		_, err := fn.Handle(ctx, testCommand{value: "hello"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "v", capturedCtx.Value(ctxKey("k")))
		assert.Equal(t, "hello", capturedCmd.value)
	})
}

// ---------------------------------------------------------------------------
// QueryHandlerFunc
// ---------------------------------------------------------------------------

func TestQueryHandlerFunc(t *testing.T) {
	t.Parallel()

	t.Run("delegates to underlying function on success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		want := 42
		fn := cqrs.QueryHandlerFunc[testQuery, int](func(_ context.Context, _ testQuery) (int, error) {
			return want, nil
		})

		// Act
		got, err := fn.Handle(context.Background(), testQuery{})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("propagates error returned by underlying function", func(t *testing.T) {
		t.Parallel()

		// Arrange
		sentinel := errors.New("query error")
		fn := cqrs.QueryHandlerFunc[testQuery, int](func(_ context.Context, _ testQuery) (int, error) {
			return 0, sentinel
		})

		// Act
		_, err := fn.Handle(context.Background(), testQuery{})

		// Assert
		require.ErrorIs(t, err, sentinel)
	})
}

// ---------------------------------------------------------------------------
// ChainCommand
// ---------------------------------------------------------------------------

func TestChainCommand(t *testing.T) {
	t.Parallel()

	t.Run("returns original handler when no middlewares provided", func(t *testing.T) {
		t.Parallel()

		// Arrange
		base := cqrs.CommandHandlerFunc[testCommand, string](func(_ context.Context, cmd testCommand) (string, error) {
			return cmd.value, nil
		})

		// Act
		chained := cqrs.ChainCommand[testCommand, string](base)
		got, err := chained.Handle(context.Background(), testCommand{value: "x"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "x", got)
	})

	t.Run("applies middlewares so first middleware is outermost", func(t *testing.T) {
		t.Parallel()

		// Arrange – each middleware appends its label to an execution log so
		// we can verify the wrapping order (first in = outermost wrapper).
		var log []string

		makeMiddleware := func(label string) cqrs.CommandMiddleware[testCommand, string] {
			return func(next cqrs.CommandHandler[testCommand, string]) cqrs.CommandHandler[testCommand, string] {
				return cqrs.CommandHandlerFunc[testCommand, string](func(ctx context.Context, cmd testCommand) (string, error) {
					log = append(log, label+":before")
					res, err := next.Handle(ctx, cmd)
					log = append(log, label+":after")
					return res, err
				})
			}
		}

		base := cqrs.CommandHandlerFunc[testCommand, string](func(_ context.Context, cmd testCommand) (string, error) {
			log = append(log, "base")
			return cmd.value, nil
		})

		// Act
		chained := cqrs.ChainCommand[testCommand, string](base, makeMiddleware("A"), makeMiddleware("B"))
		_, err := chained.Handle(context.Background(), testCommand{value: "v"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, []string{"A:before", "B:before", "base", "B:after", "A:after"}, log)
	})

	t.Run("single middleware wraps correctly", func(t *testing.T) {
		t.Parallel()

		// Arrange
		called := false
		mw := func(next cqrs.CommandHandler[testCommand, string]) cqrs.CommandHandler[testCommand, string] {
			return cqrs.CommandHandlerFunc[testCommand, string](func(ctx context.Context, cmd testCommand) (string, error) {
				called = true
				return next.Handle(ctx, cmd)
			})
		}
		base := cqrs.CommandHandlerFunc[testCommand, string](func(_ context.Context, cmd testCommand) (string, error) {
			return cmd.value, nil
		})

		// Act
		chained := cqrs.ChainCommand[testCommand, string](base, mw)
		got, err := chained.Handle(context.Background(), testCommand{value: "ok"})

		// Assert
		require.NoError(t, err)
		assert.True(t, called)
		assert.Equal(t, "ok", got)
	})
}

// ---------------------------------------------------------------------------
// ChainQuery
// ---------------------------------------------------------------------------

func TestChainQuery(t *testing.T) {
	t.Parallel()

	t.Run("returns original handler when no middlewares provided", func(t *testing.T) {
		t.Parallel()

		// Arrange
		base := cqrs.QueryHandlerFunc[testQuery, int](func(_ context.Context, _ testQuery) (int, error) {
			return 7, nil
		})

		// Act
		chained := cqrs.ChainQuery[testQuery, int](base)
		got, err := chained.Handle(context.Background(), testQuery{})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, 7, got)
	})

	t.Run("applies middlewares so first middleware is outermost", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var log []string

		makeMiddleware := func(label string) cqrs.QueryMiddleware[testQuery, int] {
			return func(next cqrs.QueryHandler[testQuery, int]) cqrs.QueryHandler[testQuery, int] {
				return cqrs.QueryHandlerFunc[testQuery, int](func(ctx context.Context, q testQuery) (int, error) {
					log = append(log, label+":before")
					res, err := next.Handle(ctx, q)
					log = append(log, label+":after")
					return res, err
				})
			}
		}

		base := cqrs.QueryHandlerFunc[testQuery, int](func(_ context.Context, _ testQuery) (int, error) {
			log = append(log, "base")
			return 1, nil
		})

		// Act
		chained := cqrs.ChainQuery[testQuery, int](base, makeMiddleware("X"), makeMiddleware("Y"))
		_, err := chained.Handle(context.Background(), testQuery{})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, []string{"X:before", "Y:before", "base", "Y:after", "X:after"}, log)
	})
}
