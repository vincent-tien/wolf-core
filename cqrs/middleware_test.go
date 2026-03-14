package cqrs_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/vincent-tien/wolf-core/cqrs"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nopCommand / nopQuery reuse the fixtures defined in handler_test.go within
// the same package (cqrs_test), so we use distinct names to avoid redeclaration.

type mwCmd struct{ payload string }
type mwQuery struct{ key string }

// succeedCmd returns a CommandHandlerFunc that always succeeds.
func succeedCmd(result string) cqrs.CommandHandler[mwCmd, string] {
	return cqrs.CommandHandlerFunc[mwCmd, string](func(_ context.Context, _ mwCmd) (string, error) {
		return result, nil
	})
}

// failCmd returns a CommandHandlerFunc that always returns the given error.
func failCmd(err error) cqrs.CommandHandler[mwCmd, string] {
	return cqrs.CommandHandlerFunc[mwCmd, string](func(_ context.Context, _ mwCmd) (string, error) {
		return "", err
	})
}

// succeedQuery returns a QueryHandlerFunc that always succeeds.
func succeedQuery(result string) cqrs.QueryHandler[mwQuery, string] {
	return cqrs.QueryHandlerFunc[mwQuery, string](func(_ context.Context, _ mwQuery) (string, error) {
		return result, nil
	})
}

// ---------------------------------------------------------------------------
// WithCommandLogging
// ---------------------------------------------------------------------------

func TestWithCommandLogging(t *testing.T) {
	t.Parallel()

	t.Run("logs and returns result on success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		logger := zaptest.NewLogger(t)
		mw := cqrs.WithCommandLogging[mwCmd, string](logger, "DoSomething")
		handler := mw(succeedCmd("ok"))

		// Act
		got, err := handler.Handle(context.Background(), mwCmd{payload: "p"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "ok", got)
	})

	t.Run("logs and propagates error on failure", func(t *testing.T) {
		t.Parallel()

		// Arrange
		logger := zaptest.NewLogger(t)
		sentinel := errors.New("boom")
		mw := cqrs.WithCommandLogging[mwCmd, string](logger, "DoSomething")
		handler := mw(failCmd(sentinel))

		// Act
		_, err := handler.Handle(context.Background(), mwCmd{})

		// Assert
		require.ErrorIs(t, err, sentinel)
	})
}

// ---------------------------------------------------------------------------
// WithCommandValidation
// ---------------------------------------------------------------------------

func TestWithCommandValidation(t *testing.T) {
	t.Parallel()

	t.Run("calls handler when validation passes", func(t *testing.T) {
		t.Parallel()

		// Arrange
		called := false
		base := cqrs.CommandHandlerFunc[mwCmd, string](func(_ context.Context, _ mwCmd) (string, error) {
			called = true
			return "done", nil
		})
		mw := cqrs.WithCommandValidation[mwCmd, string](func(_ mwCmd) error { return nil })

		// Act
		got, err := mw(base).Handle(context.Background(), mwCmd{})

		// Assert
		require.NoError(t, err)
		assert.True(t, called)
		assert.Equal(t, "done", got)
	})

	t.Run("short-circuits and returns validation error without calling handler", func(t *testing.T) {
		t.Parallel()

		// Arrange
		validationErr := errors.New("invalid payload")
		handlerCalled := false

		base := cqrs.CommandHandlerFunc[mwCmd, string](func(_ context.Context, _ mwCmd) (string, error) {
			handlerCalled = true
			return "", nil
		})
		mw := cqrs.WithCommandValidation[mwCmd, string](func(_ mwCmd) error { return validationErr })

		// Act
		_, err := mw(base).Handle(context.Background(), mwCmd{})

		// Assert
		require.ErrorIs(t, err, validationErr)
		assert.False(t, handlerCalled, "handler must not be called when validation fails")
	})
}

// ---------------------------------------------------------------------------
// WithCommandMetrics
// ---------------------------------------------------------------------------

// newIsolatedRegistry builds Histogram+Counter pairs on an isolated registry
// so tests can run in parallel without conflicting with the default registry.
func newIsolatedRegistry(t *testing.T) (
	reg *prometheus.Registry,
	duration *prometheus.HistogramVec,
	total *prometheus.CounterVec,
) {
	t.Helper()

	reg = prometheus.NewRegistry()

	duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "test_cmd_duration_seconds",
			Help:    "test",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"command_name", "status"},
	)
	total = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_cmd_total",
			Help: "test",
		},
		[]string{"command_name", "status"},
	)

	reg.MustRegister(duration, total)
	return reg, duration, total
}

// counterValue reads the current value of the counter labelled with the
// given label pairs from the collector.
func counterValue(t *testing.T, c *prometheus.CounterVec, labels prometheus.Labels) float64 {
	t.Helper()

	m := &dto.Metric{}
	require.NoError(t, c.With(labels).Write(m))

	return m.GetCounter().GetValue()
}

func TestWithCommandMetrics(t *testing.T) {
	t.Parallel()

	t.Run("records success status when handler succeeds", func(t *testing.T) {
		t.Parallel()

		// Arrange
		_, duration, total := newIsolatedRegistry(t)
		mw := cqrs.WithCommandMetrics[mwCmd, string](duration, total, "CreateOrder")
		handler := mw(succeedCmd("ok"))

		// Act
		_, err := handler.Handle(context.Background(), mwCmd{})

		// Assert
		require.NoError(t, err)
		v := counterValue(t, total, prometheus.Labels{"command_name": "CreateOrder", "status": "success"})
		assert.Equal(t, float64(1), v)
	})

	t.Run("records error status when handler fails", func(t *testing.T) {
		t.Parallel()

		// Arrange
		_, duration, total := newIsolatedRegistry(t)
		mw := cqrs.WithCommandMetrics[mwCmd, string](duration, total, "CreateOrder")
		handler := mw(failCmd(errors.New("db error")))

		// Act
		_, _ = handler.Handle(context.Background(), mwCmd{})

		// Assert
		v := counterValue(t, total, prometheus.Labels{"command_name": "CreateOrder", "status": "error"})
		assert.Equal(t, float64(1), v)
	})

	t.Run("increments counter on each invocation", func(t *testing.T) {
		t.Parallel()

		// Arrange
		_, duration, total := newIsolatedRegistry(t)
		mw := cqrs.WithCommandMetrics[mwCmd, string](duration, total, "PlaceOrder")
		handler := mw(succeedCmd("ok"))
		ctx := context.Background()

		// Act
		const calls = 5
		for i := 0; i < calls; i++ {
			_, err := handler.Handle(ctx, mwCmd{})
			require.NoError(t, err)
		}

		// Assert
		v := counterValue(t, total, prometheus.Labels{"command_name": "PlaceOrder", "status": "success"})
		assert.Equal(t, float64(calls), v)
	})

	t.Run("duration histogram has observation after call", func(t *testing.T) {
		t.Parallel()

		// Arrange
		reg, duration, total := newIsolatedRegistry(t)
		mw := cqrs.WithCommandMetrics[mwCmd, string](duration, total, "FastCmd")
		handler := mw(succeedCmd("v"))

		// Act
		_, err := handler.Handle(context.Background(), mwCmd{})

		// Assert — gather from the isolated registry and verify sample count.
		require.NoError(t, err)

		families, gatherErr := reg.Gather()
		require.NoError(t, gatherErr)

		var sampleCount uint64
		for _, fam := range families {
			if fam.GetName() != "test_cmd_duration_seconds" {
				continue
			}
			for _, m := range fam.GetMetric() {
				sampleCount += m.GetHistogram().GetSampleCount()
			}
		}
		assert.Equal(t, uint64(1), sampleCount)
	})
}

// ---------------------------------------------------------------------------
// WithQueryLogging
// ---------------------------------------------------------------------------

func TestWithQueryLogging(t *testing.T) {
	t.Parallel()

	t.Run("logs and returns result on success", func(t *testing.T) {
		t.Parallel()

		// Arrange
		logger := zaptest.NewLogger(t)
		mw := cqrs.WithQueryLogging[mwQuery, string](logger, "GetProduct")
		handler := mw(succeedQuery("product"))

		// Act
		got, err := handler.Handle(context.Background(), mwQuery{key: "k"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "product", got)
	})

	t.Run("logs and propagates error on failure", func(t *testing.T) {
		t.Parallel()

		// Arrange
		logger := zaptest.NewLogger(t)
		sentinel := errors.New("not found")
		failHandler := cqrs.QueryHandlerFunc[mwQuery, string](func(_ context.Context, _ mwQuery) (string, error) {
			return "", sentinel
		})
		mw := cqrs.WithQueryLogging[mwQuery, string](logger, "GetProduct")

		// Act
		_, err := mw(failHandler).Handle(context.Background(), mwQuery{})

		// Assert
		require.ErrorIs(t, err, sentinel)
	})
}

// ---------------------------------------------------------------------------
// WithQueryCaching
// ---------------------------------------------------------------------------

// inMemoryCache is a simple goroutine-safe stub that implements CacheGetter[R].
type inMemoryCache[R any] struct {
	mu      sync.RWMutex
	store   map[string]R
	getErr  error // returned by Get when non-nil
	setErr  error // returned by Set when non-nil
	getCalls int
	setCalls int
}

func newInMemoryCache[R any]() *inMemoryCache[R] {
	return &inMemoryCache[R]{store: make(map[string]R)}
}

func (c *inMemoryCache[R]) Get(_ context.Context, key string) (R, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.getCalls++
	if c.getErr != nil {
		var zero R
		return zero, false, c.getErr
	}
	v, ok := c.store[key]
	return v, ok, nil
}

func (c *inMemoryCache[R]) Set(_ context.Context, key string, value R, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.setCalls++
	if c.setErr != nil {
		return c.setErr
	}
	c.store[key] = value
	return nil
}

func TestWithQueryCaching(t *testing.T) {
	t.Parallel()

	keyFn := func(q mwQuery) string { return q.key }
	ttl := time.Minute

	t.Run("returns cached value on cache hit without calling handler", func(t *testing.T) {
		t.Parallel()

		// Arrange
		logger := zap.NewNop()
		cache := newInMemoryCache[string]()
		cache.store["myKey"] = "cachedValue"

		handlerCalled := false
		base := cqrs.QueryHandlerFunc[mwQuery, string](func(_ context.Context, _ mwQuery) (string, error) {
			handlerCalled = true
			return "freshValue", nil
		})

		mw := cqrs.WithQueryCaching[mwQuery, string](cache, keyFn, ttl, logger)

		// Act
		got, err := mw(base).Handle(context.Background(), mwQuery{key: "myKey"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "cachedValue", got)
		assert.False(t, handlerCalled, "handler must not be called on a cache hit")
	})

	t.Run("calls handler on cache miss and stores result", func(t *testing.T) {
		t.Parallel()

		// Arrange
		logger := zap.NewNop()
		cache := newInMemoryCache[string]()
		base := succeedQuery("freshValue")
		mw := cqrs.WithQueryCaching[mwQuery, string](cache, keyFn, ttl, logger)

		// Act
		got, err := mw(base).Handle(context.Background(), mwQuery{key: "missKey"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "freshValue", got)
		assert.Equal(t, "freshValue", cache.store["missKey"], "result must be stored in cache")
		assert.Equal(t, 1, cache.setCalls)
	})

	t.Run("does not fail query when cache Get returns error", func(t *testing.T) {
		t.Parallel()

		// Arrange
		logger := zap.NewNop()
		cache := newInMemoryCache[string]()
		cache.getErr = errors.New("redis unavailable")

		base := succeedQuery("fallback")
		mw := cqrs.WithQueryCaching[mwQuery, string](cache, keyFn, ttl, logger)

		// Act
		got, err := mw(base).Handle(context.Background(), mwQuery{key: "k"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "fallback", got)
	})

	t.Run("does not fail query when cache Set returns error", func(t *testing.T) {
		t.Parallel()

		// Arrange
		logger := zap.NewNop()
		cache := newInMemoryCache[string]()
		cache.setErr = errors.New("redis write failure")

		base := succeedQuery("value")
		mw := cqrs.WithQueryCaching[mwQuery, string](cache, keyFn, ttl, logger)

		// Act
		got, err := mw(base).Handle(context.Background(), mwQuery{key: "k"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "value", got)
	})

	t.Run("propagates handler error without caching", func(t *testing.T) {
		t.Parallel()

		// Arrange
		logger := zap.NewNop()
		cache := newInMemoryCache[string]()
		sentinel := errors.New("handler error")
		failHandler := cqrs.QueryHandlerFunc[mwQuery, string](func(_ context.Context, _ mwQuery) (string, error) {
			return "", sentinel
		})
		mw := cqrs.WithQueryCaching[mwQuery, string](cache, keyFn, ttl, logger)

		// Act
		_, err := mw(failHandler).Handle(context.Background(), mwQuery{key: "k"})

		// Assert
		require.ErrorIs(t, err, sentinel)
		assert.Equal(t, 0, cache.setCalls, "Set must not be called when handler fails")
	})

	t.Run("second call hits cache after first miss populates it", func(t *testing.T) {
		t.Parallel()

		// Arrange
		logger := zap.NewNop()
		cache := newInMemoryCache[string]()
		callCount := 0
		base := cqrs.QueryHandlerFunc[mwQuery, string](func(_ context.Context, _ mwQuery) (string, error) {
			callCount++
			return "computed", nil
		})
		mw := cqrs.WithQueryCaching[mwQuery, string](cache, keyFn, ttl, logger)
		handler := mw(base)
		q := mwQuery{key: "repeatKey"}

		// Act
		got1, err1 := handler.Handle(context.Background(), q)
		got2, err2 := handler.Handle(context.Background(), q)

		// Assert
		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.Equal(t, "computed", got1)
		assert.Equal(t, "computed", got2)
		assert.Equal(t, 1, callCount, "underlying handler must only be called once")
	})
}
