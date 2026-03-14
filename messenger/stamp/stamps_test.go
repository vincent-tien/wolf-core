package stamp_test

import (
	"testing"
	"time"

	"github.com/vincent-tien/wolf-core/messenger/stamp"
)

func TestStampNames(t *testing.T) {
	tests := []struct {
		stamp stamp.Stamp
		want  string
	}{
		{stamp.BusNameStamp{Name: "default"}, stamp.NameBusName},
		{stamp.ResultStamp{Value: 42}, stamp.NameResult},
		{stamp.SentStamp{Transport: "nats", SentAt: time.Now()}, stamp.NameSent},
		{stamp.ReceivedStamp{Transport: "nats", ReceivedAt: time.Now()}, stamp.NameReceived},
		{stamp.TransportNameStamp{Name: "async"}, stamp.NameTransportName},
		{stamp.RedeliveryStamp{RetryCount: 3, LastError: "timeout"}, stamp.NameRedelivery},
		{stamp.DelayStamp{Duration: 5 * time.Second}, stamp.NameDelay},
		{stamp.ErrorStamp{Err: "fail", OccurredAt: time.Now()}, stamp.NameError},
		{stamp.TraceStamp{TraceID: "abc", SpanID: "def"}, stamp.NameTrace},
		{stamp.ForceSyncStamp{}, stamp.NameForceSync},
		{stamp.ForceTransportStamp{TransportName: "kafka"}, stamp.NameForceTransport},
		{stamp.ConsumedByStamp{Handler: "h1", Duration: time.Millisecond}, stamp.NameConsumed},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.stamp.StampName(); got != tt.want {
				t.Errorf("StampName() = %q, want %q", got, tt.want)
			}
		})
	}
}
