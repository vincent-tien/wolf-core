package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHasRole(t *testing.T) {
	c := &UserClaims{Roles: []string{"admin", "editor"}}
	assert.True(t, c.HasRole("admin"))
	assert.True(t, c.HasRole("editor"))
	assert.False(t, c.HasRole("viewer"))
}

func TestHasPermission(t *testing.T) {
	c := &UserClaims{Permissions: []string{"product:read", "product:write"}}
	assert.True(t, c.HasPermission("product:read"))
	assert.False(t, c.HasPermission("order:read"))
}

func TestHasAnyRole(t *testing.T) {
	c := &UserClaims{Roles: []string{"editor"}}
	assert.True(t, c.HasAnyRole("admin", "editor"))
	assert.False(t, c.HasAnyRole("admin", "viewer"))
}

func TestHasAllPermissions(t *testing.T) {
	c := &UserClaims{Permissions: []string{"product:read", "product:write", "order:read"}}
	assert.True(t, c.HasAllPermissions("product:read", "product:write"))
	assert.False(t, c.HasAllPermissions("product:read", "user:admin"))
}

func TestIsExpired(t *testing.T) {
	now := time.Now()
	c := &UserClaims{ExpiresAt: now.Add(time.Hour)}
	assert.False(t, c.IsExpired(now))
	assert.True(t, c.IsExpired(now.Add(2*time.Hour)))
}
