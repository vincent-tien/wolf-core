// trace.go — Stamp carrying OpenTelemetry trace context across async boundaries.
package stamp

// TraceStamp carries OpenTelemetry trace context across async boundaries.
//
// Headers holds full W3C propagation headers (traceparent, tracestate) for
// cross-process correlation. TraceID/SpanID are kept for backward compatibility
// with envelopes serialized before Headers was added.
type TraceStamp struct {
	TraceID string            `json:"trace_id,omitempty"`
	SpanID  string            `json:"span_id,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (TraceStamp) StampName() string { return NameTrace }
