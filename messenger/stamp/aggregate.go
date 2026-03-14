// aggregate.go — Stamp carrying aggregate type/ID for outbox row correlation.
package stamp

// AggregateStamp carries aggregate metadata through the envelope.
// Used by the outbox transport to populate aggregate_type and aggregate_id
// in the outbox_events row.
type AggregateStamp struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

func (AggregateStamp) StampName() string { return NameAggregate }
