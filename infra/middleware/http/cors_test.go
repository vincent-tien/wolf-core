package http_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	httpmw "github.com/vincent-tien/wolf-core/infra/middleware/http"
)

func TestCORS_AllowCredentialsWithWildcard_Panics(t *testing.T) {
	assert.Panics(t, func() {
		httpmw.CORS(httpmw.CORSConfig{
			AllowCredentials: true,
			AllowedOrigins:   []string{"*"},
		})
	}, "AllowCredentials=true with wildcard must panic")
}

func TestCORS_AllowCredentialsWithWildcard_EmptyOriginsFallback_Panics(t *testing.T) {
	assert.Panics(t, func() {
		httpmw.CORS(httpmw.CORSConfig{
			AllowCredentials: true,
			// empty AllowedOrigins defaults to ["*"]
		})
	}, "AllowCredentials=true with empty origins (defaults to *) must panic")
}

func TestCORS_AllowCredentialsWithExplicitOrigins_NoPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		httpmw.CORS(httpmw.CORSConfig{
			AllowCredentials: true,
			AllowedOrigins:   []string{"https://example.com"},
		})
	})
}

func TestCORS_NoCredentialsWithWildcard_NoPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		httpmw.CORS(httpmw.CORSConfig{
			AllowCredentials: false,
			AllowedOrigins:   []string{"*"},
		})
	})
}
