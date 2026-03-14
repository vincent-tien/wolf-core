// cors.go — CORS configuration middleware using gin-contrib/cors.
package http

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORSConfig holds cross-origin resource sharing settings used to configure
// the CORS middleware.
type CORSConfig struct {
	// AllowedOrigins is the list of origins permitted to make cross-site
	// requests. Use ["*"] to allow all origins.
	AllowedOrigins []string
	// AllowedMethods is the list of HTTP methods allowed for cross-origin
	// requests (e.g. ["GET","POST","PUT","DELETE"]).
	AllowedMethods []string
	// AllowedHeaders is the list of non-simple request headers clients may
	// include (e.g. ["Authorization","Content-Type"]).
	AllowedHeaders []string
	// AllowCredentials indicates whether the browser should include credentials
	// (cookies, TLS certificates, HTTP authentication) with cross-origin requests.
	AllowCredentials bool
	// MaxAge specifies how long (in seconds) the result of a preflight request
	// can be cached by the browser.
	MaxAge int
}

// CORS returns a Gin middleware that applies the given CORSConfig using the
// gin-contrib/cors library. When AllowedOrigins is empty, all origins are
// permitted. MaxAge is converted from seconds to a time.Duration.
//
// It panics at startup if AllowCredentials is true and AllowedOrigins contains
// "*", because browsers silently ignore credentials with wildcard origins.
func CORS(cfg CORSConfig) gin.HandlerFunc {
	allowed := cfg.AllowedOrigins
	if len(allowed) == 0 {
		allowed = []string{"*"}
	}

	if cfg.AllowCredentials {
		for _, o := range allowed {
			if o == "*" {
				panic("cors: AllowCredentials=true is incompatible with wildcard origin \"*\"; " +
					"list explicit origins instead")
			}
		}
	}

	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	}

	headers := cfg.AllowedHeaders
	if len(headers) == 0 {
		headers = []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"}
	}

	corsCfg := cors.Config{
		AllowOrigins:     allowed,
		AllowMethods:     methods,
		AllowHeaders:     headers,
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           time.Duration(cfg.MaxAge) * time.Second,
	}

	return cors.New(corsCfg)
}
