package httputil_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/http/httputil"
)

type testRequest struct {
	Name  string `json:"name"  validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

func TestBindAndValidate(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(
			http.MethodPost, "/",
			bytes.NewBufferString(`{"name":"test","email":"test@example.com"}`),
		)
		c.Request.Header.Set("Content-Type", "application/json")

		req, ok := httputil.BindAndValidate[testRequest](c)

		assert.True(t, ok)
		assert.Equal(t, "test", req.Name)
		assert.Equal(t, "test@example.com", req.Email)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("malformed JSON returns 400", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(
			http.MethodPost, "/",
			bytes.NewBufferString(`{invalid}`),
		)
		c.Request.Header.Set("Content-Type", "application/json")

		_, ok := httputil.BindAndValidate[testRequest](c)

		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var body struct {
			Error httputil.ErrorBody `json:"error"`
		}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, "VALIDATION", body.Error.Code)
	})

	t.Run("invalid email fails validation", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(
			http.MethodPost, "/",
			bytes.NewBufferString(`{"name":"test","email":"not-an-email"}`),
		)
		c.Request.Header.Set("Content-Type", "application/json")

		_, ok := httputil.BindAndValidate[testRequest](c)

		assert.False(t, ok)
		assert.NotEqual(t, http.StatusOK, w.Code)
	})

	t.Run("missing required field", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(
			http.MethodPost, "/",
			bytes.NewBufferString(`{"email":"test@example.com"}`),
		)
		c.Request.Header.Set("Content-Type", "application/json")

		_, ok := httputil.BindAndValidate[testRequest](c)

		assert.False(t, ok)
		assert.NotEqual(t, http.StatusOK, w.Code)
	})
}
