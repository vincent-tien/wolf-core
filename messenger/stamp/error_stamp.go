// error_stamp.go — Stamp recording a processing error on the envelope.
package stamp

import "time"

// ErrorStamp records a processing error on the envelope.
type ErrorStamp struct {
	Err        string
	OccurredAt time.Time
}

func (ErrorStamp) StampName() string { return NameError }
