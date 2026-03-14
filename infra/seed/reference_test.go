package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReferenceStore_SetAndGet(t *testing.T) {
	rs := NewReferenceStore()

	rs.Set("role.admin", "admin-uuid")
	val, ok := rs.Get("role.admin")

	assert.True(t, ok)
	assert.Equal(t, "admin-uuid", val)
}

func TestReferenceStore_Get_Missing(t *testing.T) {
	rs := NewReferenceStore()

	val, ok := rs.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, val)
}

func TestReferenceStore_MustGet_Panics(t *testing.T) {
	rs := NewReferenceStore()

	assert.Panics(t, func() {
		rs.MustGet("missing")
	})
}

func TestReferenceStore_Keys(t *testing.T) {
	rs := NewReferenceStore()
	rs.Set("a", 1)
	rs.Set("b", 2)
	rs.Set("c", 3)

	keys := rs.Keys()
	assert.Len(t, keys, 3)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, keys)
}

func TestGetRef_TypedAccess(t *testing.T) {
	rs := NewReferenceStore()
	rs.Set("count", 42)

	val, err := GetRef[int](rs, "count")
	require.NoError(t, err)
	assert.Equal(t, 42, val)
}

func TestGetRef_Missing(t *testing.T) {
	rs := NewReferenceStore()

	_, err := GetRef[int](rs, "missing")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGetRef_WrongType(t *testing.T) {
	rs := NewReferenceStore()
	rs.Set("count", "not-an-int")

	_, err := GetRef[int](rs, "count")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has type")
}

func TestMustGetRef_Panics_OnMissing(t *testing.T) {
	rs := NewReferenceStore()

	assert.Panics(t, func() {
		MustGetRef[int](rs, "missing")
	})
}

func TestMustGetRef_Panics_OnWrongType(t *testing.T) {
	rs := NewReferenceStore()
	rs.Set("val", "string")

	assert.Panics(t, func() {
		MustGetRef[int](rs, "val")
	})
}

func TestGetRef_SliceType(t *testing.T) {
	rs := NewReferenceStore()
	rs.Set("ids", []string{"a", "b", "c"})

	val, err := GetRef[[]string](rs, "ids")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, val)
}

func TestGetRef_StructType(t *testing.T) {
	type roleRef struct {
		ID   string
		Name string
	}
	rs := NewReferenceStore()
	rs.Set("admin", roleRef{ID: "uuid-1", Name: "admin"})

	val, err := GetRef[roleRef](rs, "admin")
	require.NoError(t, err)
	assert.Equal(t, "uuid-1", val.ID)
	assert.Equal(t, "admin", val.Name)
}
