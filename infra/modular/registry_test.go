package modular

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/event"
	sharedevent "github.com/vincent-tien/wolf-core/event"
	"github.com/vincent-tien/wolf-core/runtime"
)

// stubModule is a minimal runtime.Module for testing.
type stubModule struct {
	name string
	deps []string
}

var _ runtime.Module = (*stubModule)(nil)

func (m *stubModule) Name() string                                { return m.name }
func (m *stubModule) RegisterEvents(_ *sharedevent.TypeRegistry)  {}
func (m *stubModule) RegisterHTTP(_ interface{})                  {}
func (m *stubModule) RegisterGRPC(_ interface{})                  {}
func (m *stubModule) RegisterSubscribers(_ event.Subscriber) error { return nil }
func (m *stubModule) OnStart(_ context.Context) error             { return nil }
func (m *stubModule) OnStop(_ context.Context) error              { return nil }
func (m *stubModule) DependsOn() []string                         { return m.deps }

func TestRegistry_Register_Dedup(t *testing.T) {
	r := NewRegistry()

	err := r.Register(&stubModule{name: "iam"})
	require.NoError(t, err)

	err = r.Register(&stubModule{name: "iam"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate module")
}

func TestRegistry_Modules_NoDeps(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&stubModule{name: "a"}))
	require.NoError(t, r.Register(&stubModule{name: "b"}))

	mods, err := r.Modules()
	require.NoError(t, err)
	assert.Len(t, mods, 2)
	assert.Equal(t, "a", mods[0].Name())
	assert.Equal(t, "b", mods[1].Name())
}

func TestRegistry_Modules_WithDeps(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&stubModule{name: "order", deps: []string{"iam"}}))
	require.NoError(t, r.Register(&stubModule{name: "iam"}))

	mods, err := r.Modules()
	require.NoError(t, err)
	require.Len(t, mods, 2)
	assert.Equal(t, "iam", mods[0].Name())
	assert.Equal(t, "order", mods[1].Name())
}

func TestRegistry_Modules_CycleDetected(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&stubModule{name: "a", deps: []string{"b"}}))
	require.NoError(t, r.Register(&stubModule{name: "b", deps: []string{"a"}}))

	_, err := r.Modules()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dependency cycle")
}

func TestRegistry_Modules_MissingDep(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(&stubModule{name: "order", deps: []string{"missing"}}))

	_, err := r.Modules()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unregistered module")
}
