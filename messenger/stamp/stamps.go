// Package stamp defines metadata stamps attached to message envelopes.
package stamp

// Stamp is metadata attached to an Envelope during dispatch.
type Stamp interface {
	StampName() string
}

// Well-known stamp names.
const (
	NameBusName        = "messenger.bus_name"
	NameResult         = "messenger.result"
	NameSent           = "messenger.sent"
	NameReceived       = "messenger.received"
	NameTransportName  = "messenger.transport_name"
	NameRedelivery     = "messenger.redelivery"
	NameDelay          = "messenger.delay"
	NameError          = "messenger.error"
	NameTrace          = "messenger.trace"
	NameForceSync      = "messenger.force_sync"
	NameForceTransport = "messenger.force_transport"
	NameConsumed       = "messenger.consumed"
	NameAggregate      = "messenger.aggregate"
	NameOutboxID       = "messenger.outbox_id"
)
