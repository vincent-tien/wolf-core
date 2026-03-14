package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testPayload struct {
	SKU  string `json:"sku"`
	Name string `json:"name"`
}

func TestTypeRegistry_Register_And_Build(t *testing.T) {
	reg := NewTypeRegistry()
	reg.Register("product.created.v1", &testPayload{})

	instance, err := reg.Build("product.created.v1")
	require.NoError(t, err)

	_, ok := instance.(*testPayload)
	assert.True(t, ok, "Build must return a pointer to the registered type")
}

func TestTypeRegistry_Build_UnregisteredType_ReturnsError(t *testing.T) {
	reg := NewTypeRegistry()

	_, err := reg.Build("unknown.type")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestTypeRegistry_Register_DuplicateSameType_IsIdempotent(t *testing.T) {
	reg := NewTypeRegistry()
	reg.Register("product.created.v1", &testPayload{})

	assert.NotPanics(t, func() {
		reg.Register("product.created.v1", &testPayload{})
	})
}

func TestTypeRegistry_Register_DuplicateDifferentType_Panics(t *testing.T) {
	type otherPayload struct{ X int }
	reg := NewTypeRegistry()
	reg.Register("product.created.v1", &testPayload{})

	assert.Panics(t, func() {
		reg.Register("product.created.v1", &otherPayload{})
	})
}

func TestTypeRegistry_Serialize_And_Deserialize(t *testing.T) {
	reg := NewTypeRegistry()
	reg.Register("product.created.v1", &testPayload{})

	original := &testPayload{SKU: "ABC-123", Name: "Widget"}

	data, err := reg.Serialize("product.created.v1", original)
	require.NoError(t, err)
	assert.Contains(t, string(data), "ABC-123")

	result, err := reg.Deserialize("product.created.v1", data)
	require.NoError(t, err)

	got, ok := result.(*testPayload)
	require.True(t, ok)
	assert.Equal(t, "ABC-123", got.SKU)
	assert.Equal(t, "Widget", got.Name)
}

func TestTypeRegistry_Serialize_UnregisteredType_ReturnsError(t *testing.T) {
	reg := NewTypeRegistry()

	_, err := reg.Serialize("unknown.type", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestTypeRegistry_Deserialize_UnregisteredType_ReturnsError(t *testing.T) {
	reg := NewTypeRegistry()

	_, err := reg.Deserialize("unknown.type", []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestTypeRegistry_IsRegistered(t *testing.T) {
	reg := NewTypeRegistry()
	reg.Register("product.created.v1", &testPayload{})

	assert.True(t, reg.IsRegistered("product.created.v1"))
	assert.False(t, reg.IsRegistered("unknown.type"))
}

func TestTypeRegistry_Register_WithNonPointer(t *testing.T) {
	reg := NewTypeRegistry()
	// Pass a value (not pointer) — should work fine
	reg.Register("test.v1", testPayload{})

	instance, err := reg.Build("test.v1")
	require.NoError(t, err)
	_, ok := instance.(*testPayload)
	assert.True(t, ok, "Build must return a pointer even when registered with a value")
}
