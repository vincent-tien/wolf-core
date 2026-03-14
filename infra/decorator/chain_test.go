package decorator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/decorator"
)

func TestChain_Empty(t *testing.T) {
	t.Parallel()

	// Arrange
	base := decorator.Func[string, string](func(_ context.Context, in string) (string, error) {
		return in + "_result", nil
	})

	// Act
	chained := decorator.Chain(base)
	got, err := chained(context.Background(), "input")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "input_result", got)
}

func TestChain_ExecutionOrder(t *testing.T) {
	t.Parallel()

	// Arrange — each middleware prepends its label so we can assert order.
	record := make([]string, 0, 4)

	makeMiddleware := func(label string) decorator.Middleware[string, string] {
		return func(next decorator.Func[string, string]) decorator.Func[string, string] {
			return func(ctx context.Context, in string) (string, error) {
				record = append(record, label+"_before")
				out, err := next(ctx, in)
				record = append(record, label+"_after")
				return out, err
			}
		}
	}

	base := decorator.Func[string, string](func(_ context.Context, in string) (string, error) {
		record = append(record, "base")
		return in, nil
	})

	// Act — first middleware should be outermost (first before, last after).
	chained := decorator.Chain(base, makeMiddleware("A"), makeMiddleware("B"))
	_, err := chained(context.Background(), "x")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, []string{"A_before", "B_before", "base", "B_after", "A_after"}, record)
}

func TestChain_PropagatesError(t *testing.T) {
	t.Parallel()

	// Arrange
	sentinel := errors.New("base error")
	base := decorator.Func[string, string](func(_ context.Context, _ string) (string, error) {
		return "", sentinel
	})

	passThrough := func(next decorator.Func[string, string]) decorator.Func[string, string] {
		return func(ctx context.Context, in string) (string, error) {
			return next(ctx, in)
		}
	}

	// Act
	chained := decorator.Chain(base, decorator.Middleware[string, string](passThrough))
	_, err := chained(context.Background(), "x")

	// Assert
	require.ErrorIs(t, err, sentinel)
}
