package decorator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/vincent-tien/wolf-core/infra/decorator"
)

func newObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

func TestWithLogging_Success(t *testing.T) {
	t.Parallel()

	// Arrange
	logger, logs := newObservedLogger(t)
	fn := decorator.Func[string, string](func(_ context.Context, in string) (string, error) {
		return in + "_ok", nil
	})
	wrapped := decorator.Chain(fn, decorator.WithLogging[string, string](logger, "GetItem"))

	// Act
	got, err := wrapped(context.Background(), "x")

	// Assert
	require.NoError(t, err)
	assert.Equal(t, "x_ok", got)

	require.Equal(t, 1, logs.Len(), "expected exactly one log entry")
	entry := logs.All()[0]
	assert.Equal(t, zapcore.InfoLevel, entry.Level)
	assert.Equal(t, "operation succeeded", entry.Message)

	fieldMap := entry.ContextMap()
	assert.Equal(t, "GetItem", fieldMap["operation"])
	_, hasDuration := fieldMap["duration"]
	assert.True(t, hasDuration, "duration field must be present")
}

func TestWithLogging_Error(t *testing.T) {
	t.Parallel()

	// Arrange
	logger, logs := newObservedLogger(t)
	sentinel := errors.New("not found")
	fn := decorator.Func[string, string](func(_ context.Context, _ string) (string, error) {
		return "", sentinel
	})
	wrapped := decorator.Chain(fn, decorator.WithLogging[string, string](logger, "GetItem"))

	// Act
	_, err := wrapped(context.Background(), "missing")

	// Assert
	require.ErrorIs(t, err, sentinel)

	require.Equal(t, 1, logs.Len(), "expected exactly one log entry")
	entry := logs.All()[0]
	assert.Equal(t, zapcore.WarnLevel, entry.Level)
	assert.Equal(t, "operation failed", entry.Message)

	fieldMap := entry.ContextMap()
	assert.Equal(t, "GetItem", fieldMap["operation"])
	_, hasDuration := fieldMap["duration"]
	assert.True(t, hasDuration, "duration field must be present")
	assert.NotNil(t, fieldMap["error"], "error field must be present on failure")
}
