// Package runtime defines the lifecycle and registration contracts for
// application modules in the wolf-be platform.
package runtime

import "time"

// Policy describes the cross-cutting enforcement rules applied to a single
// HTTP or gRPC endpoint. Policies are evaluated by middleware before the
// handler is invoked.
type Policy struct {
	// RequireAuth indicates that the request must carry a valid bearer token.
	RequireAuth bool
	// Roles lists the role names that are permitted to call this endpoint.
	// An empty slice means any authenticated user is allowed.
	Roles []string
	// Permissions lists the permission slugs required for this endpoint.
	// ALL listed permissions must be present on the user's claims.
	Permissions []string
	// AllowSelf permits access if the authenticated user acts on their own resource.
	AllowSelf bool
	// SelfParamName is the URL param name for the self-check (e.g. "id").
	SelfParamName string
	// RateLimit is the maximum number of requests per second allowed per
	// client identity. Zero disables rate limiting for this endpoint.
	RateLimit int
	// MaxBodySize is the maximum allowed request body in bytes.
	// Zero means the server default applies.
	MaxBodySize int64
	// IdempotencyKey requires the client to supply an Idempotency-Key header.
	IdempotencyKey bool
	// Timeout is the maximum duration for the handler to complete.
	// Zero means no per-endpoint timeout (server-level timeout applies).
	Timeout time.Duration
	// AuditLog enables structured audit logging for this endpoint, recording
	// the caller identity, resource, and outcome.
	AuditLog bool
}

// DefaultPolicy returns a Policy with safe defaults suitable for
// general-purpose authenticated endpoints: auth required, 100 req/s rate
// limit, 1 MiB body limit, 30-second timeout, and audit logging enabled.
func DefaultPolicy() Policy {
	return Policy{
		RequireAuth: true,
		Roles:       []string{},
		RateLimit:   100,
		MaxBodySize: 1 << 20, // 1 MiB
		Timeout:     30 * time.Second,
		AuditLog:    true,
	}
}
