package spec_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/spec"
)

type testEntity struct {
	active bool
	stock  int
}

func TestNew_Satisfied(t *testing.T) {
	t.Parallel()
	s := spec.New("IsActive", func(e testEntity) bool { return e.active })

	assert.Equal(t, "IsActive", s.Name())
	assert.True(t, s.IsSatisfiedBy(testEntity{active: true}))
}

func TestNew_NotSatisfied(t *testing.T) {
	t.Parallel()
	s := spec.New("IsActive", func(e testEntity) bool { return e.active })

	assert.False(t, s.IsSatisfiedBy(testEntity{active: false}))
}

func TestCheck_ReturnsNilOnPass(t *testing.T) {
	t.Parallel()
	s := spec.New("IsActive", func(e testEntity) bool { return e.active })

	err := spec.Check(s, testEntity{active: true})
	assert.NoError(t, err)
}

func TestCheck_ReturnsViolationOnFail(t *testing.T) {
	t.Parallel()
	s := spec.New("IsActive", func(e testEntity) bool { return e.active })

	err := spec.Check(s, testEntity{active: false})
	require.Error(t, err)

	var v *spec.Violation
	require.True(t, errors.As(err, &v))
	assert.Equal(t, "IsActive", v.SpecName)
	assert.Contains(t, v.Error(), "specification")
	assert.Contains(t, v.Error(), "IsActive")
}

func TestAnd_BothSatisfied(t *testing.T) {
	t.Parallel()
	active := spec.New("IsActive", func(e testEntity) bool { return e.active })
	hasStock := spec.New("HasStock", func(e testEntity) bool { return e.stock > 0 })

	combined := spec.And(active, hasStock)

	assert.Equal(t, "IsActive AND HasStock", combined.Name())
	assert.True(t, combined.IsSatisfiedBy(testEntity{active: true, stock: 5}))
}

func TestAnd_OneFails(t *testing.T) {
	t.Parallel()
	active := spec.New("IsActive", func(e testEntity) bool { return e.active })
	hasStock := spec.New("HasStock", func(e testEntity) bool { return e.stock > 0 })

	combined := spec.And(active, hasStock)

	assert.False(t, combined.IsSatisfiedBy(testEntity{active: true, stock: 0}))
	assert.False(t, combined.IsSatisfiedBy(testEntity{active: false, stock: 5}))
}

func TestOr_EitherSatisfied(t *testing.T) {
	t.Parallel()
	active := spec.New("IsActive", func(e testEntity) bool { return e.active })
	hasStock := spec.New("HasStock", func(e testEntity) bool { return e.stock > 0 })

	combined := spec.Or(active, hasStock)

	assert.Equal(t, "IsActive OR HasStock", combined.Name())
	assert.True(t, combined.IsSatisfiedBy(testEntity{active: true, stock: 0}))
	assert.True(t, combined.IsSatisfiedBy(testEntity{active: false, stock: 5}))
}

func TestOr_BothFail(t *testing.T) {
	t.Parallel()
	active := spec.New("IsActive", func(e testEntity) bool { return e.active })
	hasStock := spec.New("HasStock", func(e testEntity) bool { return e.stock > 0 })

	combined := spec.Or(active, hasStock)

	assert.False(t, combined.IsSatisfiedBy(testEntity{active: false, stock: 0}))
}

func TestNot(t *testing.T) {
	t.Parallel()
	active := spec.New("IsActive", func(e testEntity) bool { return e.active })
	notActive := spec.Not(active)

	assert.Equal(t, "NOT IsActive", notActive.Name())
	assert.True(t, notActive.IsSatisfiedBy(testEntity{active: false}))
	assert.False(t, notActive.IsSatisfiedBy(testEntity{active: true}))
}

func TestCheckAll_AllPass(t *testing.T) {
	t.Parallel()
	active := spec.New("IsActive", func(e testEntity) bool { return e.active })
	hasStock := spec.New("HasStock", func(e testEntity) bool { return e.stock > 0 })

	errs := spec.CheckAll(testEntity{active: true, stock: 5}, active, hasStock)
	assert.Empty(t, errs)
}

func TestCheckAll_CollectsAllViolations(t *testing.T) {
	t.Parallel()
	active := spec.New("IsActive", func(e testEntity) bool { return e.active })
	hasStock := spec.New("HasStock", func(e testEntity) bool { return e.stock > 0 })

	errs := spec.CheckAll(testEntity{active: false, stock: 0}, active, hasStock)
	require.Len(t, errs, 2)

	var v1, v2 *spec.Violation
	require.True(t, errors.As(errs[0], &v1))
	require.True(t, errors.As(errs[1], &v2))
	assert.Equal(t, "IsActive", v1.SpecName)
	assert.Equal(t, "HasStock", v2.SpecName)
}

func TestFormatViolations(t *testing.T) {
	t.Parallel()
	active := spec.New("IsActive", func(e testEntity) bool { return e.active })
	hasStock := spec.New("HasStock", func(e testEntity) bool { return e.stock > 0 })

	errs := spec.CheckAll(testEntity{active: false, stock: 0}, active, hasStock)
	msg := spec.FormatViolations(errs)

	assert.Contains(t, msg, "IsActive")
	assert.Contains(t, msg, "HasStock")
	assert.Contains(t, msg, "; ")
}

func TestAnd_Nested(t *testing.T) {
	t.Parallel()
	a := spec.New("A", func(e testEntity) bool { return e.active })
	b := spec.New("B", func(e testEntity) bool { return e.stock > 0 })
	c := spec.New("C", func(e testEntity) bool { return e.stock < 100 })

	nested := spec.And(spec.And(a, b), c)

	assert.True(t, nested.IsSatisfiedBy(testEntity{active: true, stock: 50}))
	assert.False(t, nested.IsSatisfiedBy(testEntity{active: true, stock: 200}))
}
