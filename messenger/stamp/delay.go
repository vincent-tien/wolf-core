// delay.go — Stamp requesting deferred processing after a delay.
package stamp

import "time"

// DelayStamp requests deferred processing after the specified duration.
type DelayStamp struct {
	Duration time.Duration
}

func (DelayStamp) StampName() string { return NameDelay }
