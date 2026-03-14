// consumed.go — Stamp recording handler name and processing duration.
package stamp

import "time"

// ConsumedByStamp records which handler processed the message and how long it took.
type ConsumedByStamp struct {
	Handler  string
	Duration time.Duration
}

func (ConsumedByStamp) StampName() string { return NameConsumed }
