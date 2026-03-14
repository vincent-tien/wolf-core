// outbox_id.go — Stamp carrying the outbox entry ID for Ack/Reject correlation.
package stamp

// OutboxIDStamp carries the outbox entry ID for Ack/Reject correlation.
// Attached by the outbox transport's Get to each received envelope.
type OutboxIDStamp struct {
	EntryID string `json:"entry_id"`
}

func (OutboxIDStamp) StampName() string { return NameOutboxID }
