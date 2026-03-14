// typer.go — Fast message type name resolution for the messenger bus.
//
// Every message needs a canonical string name for handler lookup and routing.
// This file provides a 3-tier resolution strategy optimized for the dispatch
// hot path:
//
//  1. Typer interface (~2ns): message struct implements MessageName() string
//  2. Cached type pointer (~10ns): sync.Map lookup using unsafe type pointer
//  3. Reflect fallback (~20ns first call, then cached): only for unknown types
//
// PreregisterType[T]() is called during RegisterCommand/RegisterQuery to
// ensure the hot path never falls through to reflect. In practice, tier 1
// (Typer) is the normal path for production message structs.
package messenger

import (
	"reflect"
	"sync"
	"unsafe"
)

// Typer provides a canonical message name without reflection.
// Implement this on all message structs for optimal performance (~2ns).
type Typer interface {
	MessageName() string
}

// Versioned provides schema version for wire compatibility.
// Used by serde package for envelope serialization with schema evolution.
type Versioned interface {
	MessageVersion() int
}

// typeCache stores type pointer → name mappings for non-Typer messages.
var typeCache sync.Map // map[uintptr]string

// TypeNameOf returns the canonical name for a message.
//
// Resolution order:
//  1. Typer interface: msg.MessageName() — ~2ns, zero alloc
//  2. Cached type pointer lookup — ~10ns, zero alloc
//  3. Reflect fallback (runs once per type, result cached) — ~20ns first call
func TypeNameOf(msg any) string {
	if t, ok := msg.(Typer); ok {
		return t.MessageName()
	}
	return typeNameCached(msg)
}

// PreregisterType ensures hot path never hits reflect for type T.
// Call at startup during Register().
func PreregisterType[T any]() {
	var zero T
	rt := reflect.TypeOf(zero)
	if rt == nil {
		// interface type, use pointer to zero value
		rt = reflect.TypeOf(&zero).Elem()
	}
	key := typePointerKey(rt)
	typeCache.LoadOrStore(key, rt.String())
}

func typeNameCached(msg any) string {
	rt := reflect.TypeOf(msg)
	key := typePointerKey(rt)

	if name, ok := typeCache.Load(key); ok {
		return name.(string)
	}

	name := rt.String()
	typeCache.Store(key, name)
	return name
}

// typePointerKey extracts the runtime type pointer from reflect.Type.
// This avoids allocating reflect.Type as a map key.
func typePointerKey(rt reflect.Type) uintptr {
	// reflect.Type is an interface with two words: (type pointer, data pointer).
	// We use the data pointer which uniquely identifies the type descriptor.
	type iface struct {
		_    uintptr
		data uintptr
	}
	return (*iface)(unsafe.Pointer(&rt)).data
}
