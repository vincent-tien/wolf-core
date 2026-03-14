// wire.go — Wire format struct for versioned envelope serialization.
package serde

import (
	"encoding/json"
	"time"
)

// WireEnvelope is the ONLY format that goes on the wire.
// Explicitly versioned for schema evolution.
type WireEnvelope struct {
	SchemaVersion  int               `json:"schema_version"`
	MessageType    string            `json:"message_type"`
	MessageVersion int               `json:"message_version"`
	Payload        json.RawMessage   `json:"payload"`
	Stamps         []WireStamp       `json:"stamps"`
	ID             string            `json:"id"`
	Source         string            `json:"source"`
	CreatedAt      time.Time         `json:"created_at"`
}

// WireStamp is a serialized stamp on the wire.
type WireStamp struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}
