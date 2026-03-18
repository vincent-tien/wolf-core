package httputil_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharederrors "github.com/vincent-tien/wolf-core/errors"
	"github.com/vincent-tien/wolf-core/infra/http/httputil"
)

func init() { gin.SetMode(gin.TestMode) }

func TestError_DefaultMapper_RFC7807(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantType   string
		wantCode   string
	}{
		{
			name:       "not found",
			err:        sharederrors.NewNotFound("user", "abc"),
			wantStatus: http.StatusNotFound,
			wantType:   "/errors/not-found",
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "validation",
			err:        sharederrors.NewValidation("email", "invalid"),
			wantStatus: http.StatusBadRequest,
			wantType:   "/errors/validation",
			wantCode:   "VALIDATION",
		},
		{
			name:       "unauthorized",
			err:        sharederrors.NewUnauthorized("missing token"),
			wantStatus: http.StatusUnauthorized,
			wantType:   "/errors/unauthorized",
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "internal hides cause",
			err:        sharederrors.NewInternal(fmt.Errorf("db crashed")),
			wantStatus: http.StatusInternalServerError,
			wantType:   "/errors/internal",
			wantCode:   "INTERNAL",
		},
		{
			name:       "raw error returns 500",
			err:        fmt.Errorf("raw stdlib error"),
			wantStatus: http.StatusInternalServerError,
			wantType:   "/errors/internal",
			wantCode:   "INTERNAL",
		},
		{
			name:       "conflict",
			err:        sharederrors.NewConflict("duplicate"),
			wantStatus: http.StatusConflict,
			wantType:   "/errors/conflict",
			wantCode:   "CONFLICT",
		},
		{
			name:       "rate limited",
			err:        sharederrors.NewRateLimited(),
			wantStatus: http.StatusTooManyRequests,
			wantType:   "/errors/rate-limited",
			wantCode:   "RATE_LIMITED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users/abc", nil)

			httputil.Error(c, tt.err)

			assert.Equal(t, tt.wantStatus, w.Code)

			var body struct {
				Error httputil.ErrorBody `json:"error"`
			}
			require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
			assert.Equal(t, tt.wantType, body.Error.Type)
			assert.Equal(t, tt.wantStatus, body.Error.Status)
			assert.Equal(t, tt.wantCode, body.Error.Code)
			assert.NotEmpty(t, body.Error.Message)
			assert.Equal(t, "/api/v1/users/abc", body.Error.Instance)
		})
	}
}

func TestError_ValidationFieldPreserved(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/users", nil)

	httputil.Error(c, sharederrors.NewValidation("email", "must be valid"))

	var body struct {
		Error httputil.ErrorBody `json:"error"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "email", body.Error.Field)
}

func TestError_CustomMapper(t *testing.T) {
	t.Parallel()

	type customError struct {
		Msg  string `json:"msg"`
		Hint string `json:"hint"`
	}

	r := httputil.NewResponder(httputil.WithErrorMapper(
		func(c *gin.Context, err error) (int, any) {
			return http.StatusTeapot, &customError{Msg: err.Error(), Hint: "custom"}
		},
	))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	r.Error(c, fmt.Errorf("boom"))

	assert.Equal(t, http.StatusTeapot, w.Code)

	var body struct {
		Error customError `json:"error"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "boom", body.Error.Msg)
	assert.Equal(t, "custom", body.Error.Hint)
}

func TestOK(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	httputil.OK(c, map[string]string{"id": "1"})

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.NotNil(t, body["data"])
	assert.Nil(t, body["error"])
}

func TestCreated(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	httputil.Created(c, map[string]string{"id": "1"})

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestNoContent(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)

	httputil.NoContent(c)

	// Gin's c.Status() stores internally but doesn't flush to the recorder
	// without a body write. Check gin's internal status tracker.
	assert.Equal(t, http.StatusNoContent, c.Writer.Status())
	assert.Empty(t, w.Body.String())
}
