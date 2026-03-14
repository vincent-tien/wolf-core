// json.go — JSON serializer implementation for messenger envelopes.
package serde

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"github.com/vincent-tien/wolf-core/messenger"
	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

// JSONSerializer encodes/decodes envelopes as JSON wire format.
type JSONSerializer struct {
	registry *TypeRegistry
	source   string
}

// NewJSONSerializer creates a JSON serializer backed by the given type registry.
func NewJSONSerializer(registry *TypeRegistry, source string) *JSONSerializer {
	return &JSONSerializer{registry: registry, source: source}
}

// Encode converts an envelope to JSON wire format.
func (s *JSONSerializer) Encode(env messenger.Envelope) ([]byte, error) {
	payload, err := json.Marshal(env.Message)
	if err != nil {
		return nil, fmt.Errorf("serde: marshal payload: %w", err)
	}

	msgVersion := 1
	if v, ok := env.Message.(messenger.Versioned); ok {
		msgVersion = v.MessageVersion()
	}

	wireStamps, err := encodeStamps(env.Stamps())
	if err != nil {
		return nil, err
	}

	wire := WireEnvelope{
		SchemaVersion:  1,
		MessageType:    messenger.TypeNameOf(env.Message),
		MessageVersion: msgVersion,
		Payload:        payload,
		Stamps:         wireStamps,
		ID:             uuid.NewString(),
		Source:         s.source,
		CreatedAt:      env.CreatedAt(),
	}

	return json.Marshal(wire)
}

// Decode converts JSON wire format back to an envelope.
func (s *JSONSerializer) Decode(data []byte) (messenger.Envelope, error) {
	var wire WireEnvelope
	if err := json.Unmarshal(data, &wire); err != nil {
		return messenger.Envelope{}, fmt.Errorf("serde: unmarshal wire: %w", err)
	}

	goType, latestVersion, err := s.registry.LatestType(wire.MessageType)
	if err != nil {
		return messenger.Envelope{}, err
	}

	payload := wire.Payload
	if wire.MessageVersion < latestVersion {
		payload, err = s.registry.Upcast(wire.MessageType, payload, wire.MessageVersion, latestVersion)
		if err != nil {
			return messenger.Envelope{}, err
		}
	}

	msgPtr := reflect.New(goType)
	if err := json.Unmarshal(payload, msgPtr.Interface()); err != nil {
		return messenger.Envelope{}, fmt.Errorf("serde: unmarshal payload for %q: %w", wire.MessageType, err)
	}
	msg := msgPtr.Elem().Interface()

	stamps := decodeStamps(wire.Stamps)

	return messenger.NewEnvelopeWithTime(msg, wire.CreatedAt, stamps...), nil
}

func encodeStamps(stamps []stamp.Stamp) ([]WireStamp, error) {
	if len(stamps) == 0 {
		return nil, nil
	}
	ws := make([]WireStamp, len(stamps))
	for i, s := range stamps {
		val, err := json.Marshal(s)
		if err != nil {
			return nil, fmt.Errorf("serde: marshal stamp %q: %w", s.StampName(), err)
		}
		ws[i] = WireStamp{Name: s.StampName(), Value: val}
	}
	return ws, nil
}

// decodeStamps best-effort decodes known stamp types. Unknown stamps are skipped.
func decodeStamps(wireStamps []WireStamp) []stamp.Stamp {
	if len(wireStamps) == 0 {
		return nil
	}
	var result []stamp.Stamp
	for _, ws := range wireStamps {
		if s := decodeStamp(ws); s != nil {
			result = append(result, s)
		}
	}
	return result
}

func decodeStamp(ws WireStamp) stamp.Stamp {
	switch ws.Name {
	case stamp.NameBusName:
		var s stamp.BusNameStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameResult:
		var s stamp.ResultStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameSent:
		var s stamp.SentStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameReceived:
		var s stamp.ReceivedStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameTransportName:
		var s stamp.TransportNameStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameRedelivery:
		var s stamp.RedeliveryStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameDelay:
		var s stamp.DelayStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameError:
		var s stamp.ErrorStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameTrace:
		var s stamp.TraceStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameForceSync:
		return stamp.ForceSyncStamp{}
	case stamp.NameForceTransport:
		var s stamp.ForceTransportStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameConsumed:
		var s stamp.ConsumedByStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameAggregate:
		var s stamp.AggregateStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	case stamp.NameOutboxID:
		var s stamp.OutboxIDStamp
		if json.Unmarshal(ws.Value, &s) == nil {
			return s
		}
	}
	return nil // unknown stamp — skip
}

