package http

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/config"
)

func TestLoadShedder_AllowsBelowLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shedder := NewLoadShedder(10)

	r := gin.New()
	r.Use(shedder.Middleware())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(0), shedder.InFlight())
}

func TestLoadShedder_RejectsOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shedder := NewLoadShedder(1)

	blocked := make(chan struct{})
	release := make(chan struct{})

	r := gin.New()
	r.Use(shedder.Middleware())
	r.GET("/", func(c *gin.Context) {
		close(blocked)
		<-release
		c.Status(http.StatusOK)
	})

	// First request blocks inside handler.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		r.ServeHTTP(w, req)
	}()

	<-blocked // Wait for first request to be inside handler.

	// Second request should be shed.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	close(release)
	wg.Wait()
}

func TestLoadShedder_DecrementOnCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shedder := NewLoadShedder(100)

	var maxSeen atomic.Int64
	var wg sync.WaitGroup

	r := gin.New()
	r.Use(shedder.Middleware())
	r.GET("/", func(c *gin.Context) {
		current := shedder.InFlight()
		for {
			old := maxSeen.Load()
			if current <= old || maxSeen.CompareAndSwap(old, current) {
				break
			}
		}
		c.Status(http.StatusOK)
	})

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			r.ServeHTTP(w, req)
		}()
	}

	wg.Wait()

	assert.Equal(t, int64(0), shedder.InFlight())
	assert.LessOrEqual(t, maxSeen.Load(), int64(100))
}

func TestLoadShed_DisabledWhenZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mw := LoadShed(config.LoadShedConfig{MaxConcurrent: 0})
	require.NotNil(t, mw)

	r := gin.New()
	r.Use(mw)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
