// received.go — Stamp recording when and from which transport a message arrived.
package stamp

import "time"

// ReceivedStamp records when a message was received from a transport.
type ReceivedStamp struct {
	Transport  string
	ReceivedAt time.Time
}

func (ReceivedStamp) StampName() string { return NameReceived }
