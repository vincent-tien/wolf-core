// Package serde provides serialization for messenger envelopes on the wire.
package serde

import "github.com/vincent-tien/wolf-core/messenger"

// Serializer encodes/decodes envelopes for transport.
type Serializer interface {
	Encode(env messenger.Envelope) ([]byte, error)
	Decode(data []byte) (messenger.Envelope, error)
}
