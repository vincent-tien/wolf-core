// Package auth defines authentication and authorization contracts
// used across the entire platform. This package is import-safe
// for domain and application layers -- NO infrastructure dependencies.
package auth

import (
	"sync"
	"time"

	"github.com/vincent-tien/wolf-core/types"
)

// UserClaims represents the authenticated user's identity and permissions.
// Extracted from JWT token by the platform middleware, stored in context.
//
// Construct with NewUserClaims to get pre-built O(1) lookup sets.
// Structs zero-value-initialized (e.g. via JSON deserialization) fall back to
// lazy set initialization on first HasRole/HasPermission call.
type UserClaims struct {
	UserID      string
	Email       string
	Roles       []string
	Permissions []string
	SessionID   string
	IssuedAt    time.Time
	ExpiresAt   time.Time

	roleSet  types.Set[string]
	permSet  types.Set[string]
	initOnce sync.Once
}

// NewUserClaims constructs a UserClaims with pre-built sets for O(1) lookups.
func NewUserClaims(userID, email string, roles, permissions []string, sessionID string, issuedAt, expiresAt time.Time) *UserClaims {
	return &UserClaims{
		UserID:      userID,
		Email:       email,
		Roles:       roles,
		Permissions: permissions,
		SessionID:   sessionID,
		IssuedAt:    issuedAt,
		ExpiresAt:   expiresAt,
		roleSet:     types.NewSet(roles...),
		permSet:     types.NewSet(permissions...),
	}
}

// ensureSets lazily initializes the sets if the struct was constructed without
// NewUserClaims (e.g. via JSON deserialization). sync.Once guarantees safe
// concurrent access from multiple middleware/handler goroutines.
func (c *UserClaims) ensureSets() {
	c.initOnce.Do(func() {
		if c.roleSet == nil {
			c.roleSet = types.NewSet(c.Roles...)
		}
		if c.permSet == nil {
			c.permSet = types.NewSet(c.Permissions...)
		}
	})
}

// HasRole checks if the user has a specific role.
func (c *UserClaims) HasRole(role string) bool {
	c.ensureSets()
	return c.roleSet.Contains(role)
}

// HasPermission checks if the user has a specific permission.
func (c *UserClaims) HasPermission(perm string) bool {
	c.ensureSets()
	return c.permSet.Contains(perm)
}

// HasAnyRole checks if the user has at least one of the given roles.
func (c *UserClaims) HasAnyRole(roles ...string) bool {
	c.ensureSets()
	return c.roleSet.ContainsAny(roles...)
}

// HasAllPermissions checks if the user has ALL of the given permissions.
func (c *UserClaims) HasAllPermissions(perms ...string) bool {
	c.ensureSets()
	return c.permSet.ContainsAll(perms...)
}

// IsExpired checks if the claims have expired.
func (c *UserClaims) IsExpired(now time.Time) bool {
	return now.After(c.ExpiresAt)
}
