package seed

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testProduct struct {
	Name   string
	SKU    string
	Price  int64
	Status string
}

func TestFactory_Make(t *testing.T) {
	factory := NewFactory(func(i int, f *Faker) testProduct {
		return testProduct{
			Name:   fmt.Sprintf("Product %d", i),
			SKU:    fmt.Sprintf("SKU-%04d", i),
			Price:  f.Price(100, 10000),
			Status: "active",
		}
	})

	p := factory.Make()
	assert.Equal(t, "Product 0", p.Name)
	assert.Equal(t, "SKU-0000", p.SKU)
	assert.Equal(t, "active", p.Status)
	assert.GreaterOrEqual(t, p.Price, int64(100))
}

func TestFactory_Make_IncrementingIndex(t *testing.T) {
	factory := NewFactory(func(i int, _ *Faker) testProduct {
		return testProduct{Name: fmt.Sprintf("P%d", i)}
	})

	p0 := factory.Make()
	p1 := factory.Make()
	p2 := factory.Make()

	assert.Equal(t, "P0", p0.Name)
	assert.Equal(t, "P1", p1.Name)
	assert.Equal(t, "P2", p2.Name)
}

func TestFactory_WithState(t *testing.T) {
	factory := NewFactory(func(i int, f *Faker) testProduct {
		return testProduct{Name: "Default", Status: "active", Price: 1000}
	}).WithState(State[testProduct]{
		Name: "inactive",
		Apply: func(p *testProduct, _ int, _ *Faker) {
			p.Status = "inactive"
		},
	}).WithState(State[testProduct]{
		Name: "expensive",
		Apply: func(p *testProduct, _ int, _ *Faker) {
			p.Price = 99999
		},
	})

	// No states applied.
	p := factory.Make()
	assert.Equal(t, "active", p.Status)
	assert.Equal(t, int64(1000), p.Price)

	// Single state.
	p = factory.Make("inactive")
	assert.Equal(t, "inactive", p.Status)
	assert.Equal(t, int64(1000), p.Price)

	// Multiple states.
	p = factory.Make("inactive", "expensive")
	assert.Equal(t, "inactive", p.Status)
	assert.Equal(t, int64(99999), p.Price)
}

func TestFactory_WithState_UnknownState_Ignored(t *testing.T) {
	factory := NewFactory(func(_ int, _ *Faker) testProduct {
		return testProduct{Name: "Default"}
	})

	p := factory.Make("nonexistent")
	assert.Equal(t, "Default", p.Name)
}

func TestFactory_MakeMany(t *testing.T) {
	factory := NewFactory(func(i int, _ *Faker) testProduct {
		return testProduct{Name: fmt.Sprintf("P%d", i)}
	})

	products := factory.MakeMany(3)
	require.Len(t, products, 3)
	assert.Equal(t, "P0", products[0].Name)
	assert.Equal(t, "P1", products[1].Name)
	assert.Equal(t, "P2", products[2].Name)
}

func TestFactory_MakeMany_WithStates(t *testing.T) {
	factory := NewFactory(func(_ int, _ *Faker) testProduct {
		return testProduct{Status: "active"}
	}).WithState(State[testProduct]{
		Name:  "draft",
		Apply: func(p *testProduct, _ int, _ *Faker) { p.Status = "draft" },
	})

	products := factory.MakeMany(2, "draft")
	for _, p := range products {
		assert.Equal(t, "draft", p.Status)
	}
}

func TestFactory_MakeWithOverride(t *testing.T) {
	factory := NewFactory(func(_ int, _ *Faker) testProduct {
		return testProduct{Name: "Default", Price: 1000}
	})

	p := factory.MakeWithOverride(func(p *testProduct) {
		p.Name = "Custom"
		p.Price = 42
	})

	assert.Equal(t, "Custom", p.Name)
	assert.Equal(t, int64(42), p.Price)
}

func TestFactory_Immutability(t *testing.T) {
	base := NewFactory(func(_ int, _ *Faker) testProduct {
		return testProduct{Status: "active"}
	})

	derived := base.WithState(State[testProduct]{
		Name:  "inactive",
		Apply: func(p *testProduct, _ int, _ *Faker) { p.Status = "inactive" },
	})

	// Base should not know about "inactive" state.
	p1 := base.Make("inactive")
	assert.Equal(t, "active", p1.Status, "base factory should not have derived state")

	// Derived should.
	p2 := derived.Make("inactive")
	assert.Equal(t, "inactive", p2.Status)
}

func TestFactory_WithFaker(t *testing.T) {
	f1 := NewFaker(100)
	f2 := NewFaker(100)

	factory1 := NewFactory(func(_ int, f *Faker) testProduct {
		return testProduct{Price: f.Price(1, 1000)}
	}).WithFaker(f1)

	factory2 := NewFactory(func(_ int, f *Faker) testProduct {
		return testProduct{Price: f.Price(1, 1000)}
	}).WithFaker(f2)

	// Same faker seed should produce same products.
	assert.Equal(t, factory1.Make().Price, factory2.Make().Price)
}

func TestSequence(t *testing.T) {
	seq := NewSequence(func(n int64) string {
		return fmt.Sprintf("SKU-%04d", n)
	})

	assert.Equal(t, "SKU-0001", seq.Next())
	assert.Equal(t, "SKU-0002", seq.Next())
	assert.Equal(t, "SKU-0003", seq.Next())
}

func TestSequence_Concurrent(t *testing.T) {
	seq := NewSequence(func(n int64) int64 { return n })

	results := make(chan int64, 100)
	for range 100 {
		go func() {
			results <- seq.Next()
		}()
	}

	seen := make(map[int64]struct{})
	for range 100 {
		v := <-results
		_, exists := seen[v]
		assert.False(t, exists, "sequence should produce unique values")
		seen[v] = struct{}{}
	}
	assert.Len(t, seen, 100)
}
