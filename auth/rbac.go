// rbac.go — Role-Based Access Control contracts for the shared kernel.
//
// Only the Permission type, Authorizer interface, and RoleDefinition struct
// live here. Permission constants are owned by their respective modules
// (e.g. IAM defines "user:read", a product module would define "product:read").
// Modules pass string literals to RequirePermissions(); shared kernel does not
// need to enumerate every domain's permissions.
package auth

import "context"

// Permission represents an action on a resource.
// Format: "resource:action" e.g. "product:write", "order:read".
// Each module defines its own permission constants.
type Permission string

// RoleDefinition maps a role name to its granted permissions.
type RoleDefinition struct {
	Name        string
	Permissions []Permission
}

// Authorizer evaluates whether a user is authorized for an action.
type Authorizer interface {
	// Authorize checks if the claims satisfy the required policy.
	Authorize(ctx context.Context, claims *UserClaims, requiredRoles []string, requiredPerms []Permission) error
}
