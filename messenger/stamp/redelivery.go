// redelivery.go — Stamp tracking retry attempts for failed message processing.
package stamp

// RedeliveryStamp tracks retry attempts for failed message processing.
type RedeliveryStamp struct {
	RetryCount int
	LastError  string
}

func (RedeliveryStamp) StampName() string { return NameRedelivery }
