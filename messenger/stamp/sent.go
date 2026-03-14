// sent.go — Stamp recording when a message was sent to a transport.
package stamp

import "time"

// SentStamp records when a message was sent to a transport.
type SentStamp struct {
	Transport string
	SentAt    time.Time
}

func (SentStamp) StampName() string { return NameSent }
