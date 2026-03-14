// dispatch_result.go — Value-type result returned from Bus.Dispatch().
package messenger

// DispatchResult is returned by VALUE from Bus.Dispatch().
// Value type avoids heap allocation on the sync dispatch hot path.
//
// Callers should check Async before accessing results:
//   - Async == false: handler ran, result stamps are on the envelope
//   - Async == true:  message was sent to transport, handler runs later via worker
type DispatchResult struct {
	// Envelope holds the final state with stamps added by middleware.
	Envelope Envelope

	// Async is true if the message was sent to a transport.
	// When true, the handler has not run yet — result comes later via worker.
	Async bool
}
