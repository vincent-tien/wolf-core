package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/messaging"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// --- stub publisher ---

type stubPublisher struct {
	published []publishedCall
	err       error
}

type publishedCall struct {
	subject string
	msg     messaging.RawMessage
}

func (s *stubPublisher) Publish(_ context.Context, subject string, msg messaging.RawMessage) error {
	if s.err != nil {
		return s.err
	}
	s.published = append(s.published, publishedCall{subject: subject, msg: msg})
	return nil
}

// --- test command struct ---

type testCmd struct {
	Name  string `json:"name"  validate:"required"`
	Value int    `json:"value" validate:"min=1"`
}

// --- helpers ---

func newTestContext(method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c, w
}

// --- tests ---

func TestAsyncCommand_ValidCommand_Returns202(t *testing.T) {
	pub := &stubPublisher{}
	handler := AsyncCommand[testCmd](pub, "orders.create")

	body, err := json.Marshal(testCmd{Name: "widget", Value: 5})
	require.NoError(t, err)

	c, w := newTestContext(http.MethodPost, "/", body)
	handler(c)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp AsyncResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.RequestID)
	assert.Equal(t, "accepted", resp.Status)
	assert.Equal(t, "/api/v1/commands/"+resp.RequestID+"/status", resp.StatusURL)
}

func TestAsyncCommand_InvalidJSON_Returns400(t *testing.T) {
	pub := &stubPublisher{}
	handler := AsyncCommand[testCmd](pub, "orders.create")

	c, w := newTestContext(http.MethodPost, "/", []byte("{not valid json"))
	handler(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
	assert.Empty(t, pub.published, "nothing should be published on bind error")
}

func TestAsyncCommand_ValidationFailure_Returns400(t *testing.T) {
	pub := &stubPublisher{}
	handler := AsyncCommand[testCmd](pub, "orders.create")

	// Value must be >= 1 — send 0 to fail struct-tag validation.
	body, err := json.Marshal(testCmd{Name: "widget", Value: 0})
	require.NoError(t, err)

	c, w := newTestContext(http.MethodPost, "/", body)
	handler(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
	assert.Empty(t, pub.published, "nothing should be published on validation error")
}

func TestAsyncCommand_PublisherError_Returns500(t *testing.T) {
	pub := &stubPublisher{err: errors.New("broker unavailable")}
	handler := AsyncCommand[testCmd](pub, "orders.create")

	body, err := json.Marshal(testCmd{Name: "widget", Value: 1})
	require.NoError(t, err)

	c, w := newTestContext(http.MethodPost, "/", body)
	handler(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to submit command")
}

func TestAsyncCommand_PublishedMessageHasCorrectSubjectAndHeaders(t *testing.T) {
	const subject = "orders.create"
	pub := &stubPublisher{}
	handler := AsyncCommand[testCmd](pub, subject)

	cmd := testCmd{Name: "widget", Value: 3}
	body, err := json.Marshal(cmd)
	require.NoError(t, err)

	c, w := newTestContext(http.MethodPost, "/", body)
	handler(c)

	require.Equal(t, http.StatusAccepted, w.Code)
	require.Len(t, pub.published, 1)

	call := pub.published[0]
	assert.Equal(t, subject, call.subject)
	assert.Equal(t, subject, call.msg.Subject)
	assert.Equal(t, subject, call.msg.Name)
	assert.NotEmpty(t, call.msg.ID)
	assert.Equal(t, call.msg.ID, call.msg.Headers["request_id"])
	assert.Equal(t, subject, call.msg.Headers["command_type"])

	// The published data must unmarshal back to the original command.
	var got testCmd
	require.NoError(t, json.Unmarshal(call.msg.Data, &got))
	assert.Equal(t, cmd, got)
}

func TestAsyncCommand_RequestIDMatchesPublishedMessageID(t *testing.T) {
	pub := &stubPublisher{}
	handler := AsyncCommand[testCmd](pub, "items.delete")

	body, err := json.Marshal(testCmd{Name: "thing", Value: 2})
	require.NoError(t, err)

	c, w := newTestContext(http.MethodPost, "/", body)
	handler(c)

	require.Equal(t, http.StatusAccepted, w.Code)

	var resp AsyncResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.Len(t, pub.published, 1)
	assert.Equal(t, resp.RequestID, pub.published[0].msg.ID)
	assert.True(t, strings.HasSuffix(resp.StatusURL, resp.RequestID+"/status"))
}
