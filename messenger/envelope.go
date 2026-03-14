// envelope.go — Immutable message envelope with stamp-based metadata.
//
// Design: Envelope is a VALUE TYPE (not a pointer) so it lives on the stack
// as it passes through the middleware chain. All mutating methods (WithStamp,
// WithoutStamp) return a NEW Envelope, leaving the original untouched. This
// prevents one middleware from accidentally mutating another's view.
//
// Stamps carry cross-cutting metadata (routing, tracing, lifecycle) without
// polluting the message payload. See stamp/ for the 14 built-in stamp types.
//
// Performance: when there are 0 stamps, the stamps slice stays nil — the
// entire Envelope is stack-allocated with zero heap allocation. This matters
// on the synchronous query hot path.
package messenger

import (
	"time"

	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

// Envelope wraps a message with metadata stamps.
// Value type — passed by value between middleware for stack allocation.
// When stamps is nil (0 stamps), zero heap allocation.
type Envelope struct {
	Message   any
	stamps    []stamp.Stamp
	createdAt time.Time
}

// NewEnvelope creates an envelope wrapping msg with optional stamps.
// With 0 stamps the stamps slice stays nil — zero heap allocation.
func NewEnvelope(msg any, stamps ...stamp.Stamp) Envelope {
	var s []stamp.Stamp
	if len(stamps) > 0 {
		s = make([]stamp.Stamp, len(stamps))
		copy(s, stamps)
	}
	return Envelope{
		Message:   msg,
		stamps:    s,
		createdAt: time.Now(),
	}
}

// NewEnvelopeWithTime creates an envelope with explicit timestamp.
// Used by serde Decode to preserve original creation time from wire format.
func NewEnvelopeWithTime(msg any, t time.Time, stamps ...stamp.Stamp) Envelope {
	var s []stamp.Stamp
	if len(stamps) > 0 {
		s = make([]stamp.Stamp, len(stamps))
		copy(s, stamps)
	}
	return Envelope{
		Message:   msg,
		stamps:    s,
		createdAt: t,
	}
}

// WithStamp returns a NEW envelope with the stamp appended. Original is unchanged.
func (e Envelope) WithStamp(s stamp.Stamp) Envelope {
	newStamps := make([]stamp.Stamp, len(e.stamps)+1)
	copy(newStamps, e.stamps)
	newStamps[len(e.stamps)] = s
	return Envelope{
		Message:   e.Message,
		stamps:    newStamps,
		createdAt: e.createdAt,
	}
}

// WithoutStamp returns a NEW envelope with all stamps of the given name removed.
func (e Envelope) WithoutStamp(name string) Envelope {
	if len(e.stamps) == 0 {
		return e
	}
	filtered := make([]stamp.Stamp, 0, len(e.stamps))
	for _, s := range e.stamps {
		if s.StampName() != name {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 {
		return Envelope{Message: e.Message, createdAt: e.createdAt}
	}
	return Envelope{Message: e.Message, stamps: filtered, createdAt: e.createdAt}
}

// Last returns the last stamp with the given name, or nil if not found.
func (e Envelope) Last(name string) stamp.Stamp {
	for i := len(e.stamps) - 1; i >= 0; i-- {
		if e.stamps[i].StampName() == name {
			return e.stamps[i]
		}
	}
	return nil
}

// All returns all stamps with the given name.
func (e Envelope) All(name string) []stamp.Stamp {
	var result []stamp.Stamp
	for _, s := range e.stamps {
		if s.StampName() == name {
			result = append(result, s)
		}
	}
	return result
}

// Stamps returns a read-only copy of all stamps.
func (e Envelope) Stamps() []stamp.Stamp {
	if len(e.stamps) == 0 {
		return nil
	}
	out := make([]stamp.Stamp, len(e.stamps))
	copy(out, e.stamps)
	return out
}

// HasStamp returns true if the envelope contains a stamp with the given name.
func (e Envelope) HasStamp(name string) bool {
	for _, s := range e.stamps {
		if s.StampName() == name {
			return true
		}
	}
	return false
}

// CreatedAt returns the envelope creation timestamp.
func (e Envelope) CreatedAt() time.Time {
	return e.createdAt
}

// MessageTypeName returns the canonical type name of the wrapped message.
func (e Envelope) MessageTypeName() string {
	return TypeNameOf(e.Message)
}

// StampCount returns the number of stamps without copying.
func (e Envelope) StampCount() int {
	return len(e.stamps)
}
