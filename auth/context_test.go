package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithClaims_ClaimsFromContext_RoundTrip(t *testing.T) {
	claims := &UserClaims{UserID: "u-1", Email: "a@b.com"}
	ctx := WithClaims(context.Background(), claims)
	got := ClaimsFromContext(ctx)
	require.NotNil(t, got)
	assert.Equal(t, "u-1", got.UserID)
	assert.Equal(t, "a@b.com", got.Email)
}

func TestClaimsFromContext_NilWhenMissing(t *testing.T) {
	got := ClaimsFromContext(context.Background())
	assert.Nil(t, got)
}

func TestMustClaimsFromContext_PanicsWhenMissing(t *testing.T) {
	assert.Panics(t, func() {
		MustClaimsFromContext(context.Background())
	})
}

func TestMustClaimsFromContext_ReturnsWhenPresent(t *testing.T) {
	claims := &UserClaims{UserID: "u-2"}
	ctx := WithClaims(context.Background(), claims)
	got := MustClaimsFromContext(ctx)
	assert.Equal(t, "u-2", got.UserID)
}

func TestUserIDFromContext(t *testing.T) {
	assert.Equal(t, "", UserIDFromContext(context.Background()))

	ctx := WithClaims(context.Background(), &UserClaims{UserID: "u-3"})
	assert.Equal(t, "u-3", UserIDFromContext(ctx))
}
