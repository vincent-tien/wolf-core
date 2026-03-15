// async.go — Generic async command handler returning 202 Accepted with status URL.
package http

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vincent-tien/wolf-core/messaging"
	"github.com/vincent-tien/wolf-core/validator"
)

// AsyncResponse is the standard response for async commands.
type AsyncResponse struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url,omitempty"`
}

// AsyncCommand creates a Gin handler that:
//  1. Binds and validates the request body to type C
//  2. Publishes it as a command message to the given subject
//  3. Returns 202 Accepted with a request ID and status URL
//
// C is the command type that will be JSON-decoded from the request body.
func AsyncCommand[C any](publisher messaging.Publisher, subject string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cmd C
		if err := c.ShouldBindJSON(&cmd); err != nil {
			AbortBadRequest(c, err.Error())
			return
		}

		if err := validator.Validate(cmd); err != nil {
			AbortBadRequest(c, err.Error())
			return
		}

		requestID := uuid.New().String()

		data, err := json.Marshal(cmd)
		if err != nil {
			AbortInternalError(c, "failed to marshal command")
			return
		}

		msg := messaging.RawMessage{
			ID:      requestID,
			Name:    subject,
			Subject: subject,
			Data:    data,
			Headers: map[string]string{
				"request_id":   requestID,
				"command_type": subject,
			},
		}

		if err := publisher.Publish(c.Request.Context(), subject, msg); err != nil {
			AbortInternalError(c, "failed to submit command")
			return
		}

		Accepted(c, AsyncResponse{
			RequestID: requestID,
			Status:    "accepted",
			StatusURL: "/api/v1/commands/" + requestID + "/status",
		})
	}
}
