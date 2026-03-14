// apikey.go — API key authentication middleware with constant-time comparison.
package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

const apiKeyServiceContextKey = "api_key_service"

// APIKeyMiddleware validates service-to-service API keys from the X-API-Key header.
// Keys are stored as SHA-256 hashes in memory for security.
type APIKeyMiddleware struct {
	// keyHashes maps service name → SHA-256 hash of the API key.
	keyHashes map[string][sha256.Size]byte
}

// NewAPIKeyMiddleware creates an APIKeyMiddleware from a map of service-name → raw-key.
// Raw keys are immediately hashed and the originals are not retained.
func NewAPIKeyMiddleware(keys map[string]string) *APIKeyMiddleware {
	hashes := make(map[string][sha256.Size]byte, len(keys))
	for service, rawKey := range keys {
		hashes[service] = sha256.Sum256([]byte(rawKey))
	}
	return &APIKeyMiddleware{keyHashes: hashes}
}

// Handler returns a Gin middleware that validates the X-API-Key header.
// Uses constant-time comparison via crypto/subtle to prevent timing attacks.
// Returns 401 if the key is missing or invalid.
func (m *APIKeyMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := c.GetHeader("X-API-Key")
		if rawKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "X-API-Key header is required",
			})
			return
		}

		provided := sha256.Sum256([]byte(rawKey))

		for service, stored := range m.keyHashes {
			if subtle.ConstantTimeCompare(provided[:], stored[:]) == 1 {
				c.Set(apiKeyServiceContextKey, service)
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "invalid API key",
		})
	}
}
