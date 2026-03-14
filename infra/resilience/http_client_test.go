package resilience_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/resilience"
)

func TestResilientHTTPClient_SuccessOnFirstAttempt(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := resilience.NewResilientHTTPClient("test", 5*time.Second, 3, zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	resp, err := client.Do(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestResilientHTTPClient_RetriesOn500(t *testing.T) {
	t.Parallel()

	// Fail once then succeed. The CB stays closed because the failure ratio
	// is only 50 % after the first two requests (≤ threshold).
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := resilience.NewResilientHTTPClient("test-retry", 5*time.Second, 3, zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	resp, err := client.Do(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.GreaterOrEqual(t, attempts.Load(), int32(2))
	resp.Body.Close()
}

func TestResilientHTTPClient_ContextCancellationStopsRetries(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := resilience.NewResilientHTTPClient("test-cancel", 5*time.Second, 10, zap.NewNop())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	resp, err := client.Do(ctx, req)
	if resp != nil {
		resp.Body.Close()
	}
	assert.Error(t, err)
}

func TestResilientHTTPClient_4xxNotRetried(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := resilience.NewResilientHTTPClient("test-4xx", 5*time.Second, 3, zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	resp, err := client.Do(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	assert.Equal(t, int32(1), attempts.Load(), "4xx should not be retried")
	resp.Body.Close()
}
