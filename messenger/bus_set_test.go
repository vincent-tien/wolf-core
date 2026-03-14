package messenger_test

import (
	"context"
	"testing"

	"github.com/vincent-tien/wolf-core/messenger"
)

type cmdA struct{}

func (cmdA) MessageName() string { return "test.CmdA" }

type cmdB struct{}

func (cmdB) MessageName() string { return "test.CmdB" }

func TestBusSet_SeparateRegistries(t *testing.T) {
	cmdBus := messenger.NewBus("command")
	queryBus := messenger.NewBus("query")

	messenger.RegisterCommandFunc[cmdA](cmdBus.Handlers(), func(_ context.Context, _ cmdA) error {
		return nil
	})

	set := messenger.NewBusSet(
		messenger.NamedBus{Name: "command", Bus: cmdBus},
		messenger.NamedBus{Name: "query", Bus: queryBus},
	)

	// cmdA registered on command bus only.
	if !set.Command().Handlers().Has(cmdA{}) {
		t.Error("command bus should have cmdA handler")
	}
	if set.Query().Handlers().Has(cmdA{}) {
		t.Error("query bus should NOT have cmdA handler")
	}
}

func TestBusSet_DistinctInstances(t *testing.T) {
	cmdBus := messenger.NewBus("command")
	queryBus := messenger.NewBus("query")

	set := messenger.NewBusSet(
		messenger.NamedBus{Name: "command", Bus: cmdBus},
		messenger.NamedBus{Name: "query", Bus: queryBus},
	)

	if set.Command() == set.Query() {
		t.Error("Command() and Query() should return distinct bus instances")
	}
}

func TestBusSet_Get_Found(t *testing.T) {
	bus := messenger.NewBus("default")
	set := messenger.NewBusSet(messenger.NamedBus{Name: "default", Bus: bus})

	got, ok := set.Get("default")
	if !ok || got != bus {
		t.Error("Get should find the default bus")
	}
}

func TestBusSet_Get_NotFound(t *testing.T) {
	set := messenger.NewBusSet()
	_, ok := set.Get("missing")
	if ok {
		t.Error("Get should return false for missing bus")
	}
}

func TestBusSet_Bus_PanicsOnMissing(t *testing.T) {
	set := messenger.NewBusSet()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Bus() should panic for missing name")
		}
	}()
	set.Bus("missing")
}

func TestBusSet_Names(t *testing.T) {
	set := messenger.NewBusSet(
		messenger.NamedBus{Name: "query", Bus: messenger.NewBus("query")},
		messenger.NamedBus{Name: "command", Bus: messenger.NewBus("command")},
	)

	names := set.Names()
	if len(names) != 2 {
		t.Fatalf("Names() len = %d, want 2", len(names))
	}
	// Sorted order.
	if names[0] != "command" || names[1] != "query" {
		t.Errorf("Names() = %v, want [command query]", names)
	}
}

func TestBusSet_DuplicateName_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewBusSet should panic on duplicate names")
		}
	}()

	bus := messenger.NewBus("dup")
	messenger.NewBusSet(
		messenger.NamedBus{Name: "same", Bus: bus},
		messenger.NamedBus{Name: "same", Bus: bus},
	)
}

func TestBusSet_SharedTransport(t *testing.T) {
	// Both buses share the same sender — verifies no cross-contamination.
	shared := &noopSender{}

	cmdBus := messenger.NewBus("command", messenger.WithTransport("shared", shared))
	queryBus := messenger.NewBus("query", messenger.WithTransport("shared", shared))

	messenger.RegisterCommandFunc[cmdA](cmdBus.Handlers(), func(_ context.Context, _ cmdA) error {
		return nil
	})
	messenger.RegisterCommandFunc[cmdB](queryBus.Handlers(), func(_ context.Context, _ cmdB) error {
		return nil
	})

	set := messenger.NewBusSet(
		messenger.NamedBus{Name: "command", Bus: cmdBus},
		messenger.NamedBus{Name: "query", Bus: queryBus},
	)

	// cmdA on command bus, cmdB on query bus — no cross-leaking.
	if set.Command().Handlers().Has(cmdB{}) {
		t.Error("command bus should NOT have cmdB")
	}
	if set.Query().Handlers().Has(cmdA{}) {
		t.Error("query bus should NOT have cmdA")
	}
}

type noopSender struct{}

func (noopSender) Send(_ context.Context, _ messenger.Envelope) error { return nil }
