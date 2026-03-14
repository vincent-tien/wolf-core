// category.go — Distinguishes domain events from integration events.
package event

// Category classifies events as domain-internal or integration (cross-boundary).
type Category string

const (
	// CategoryDomain represents events that describe state transitions within a
	// single bounded context. They carry domain-specific language and types.
	CategoryDomain Category = "domain"

	// CategoryIntegration represents events designed for cross-boundary
	// consumption. They use a stable, public schema and avoid leaking
	// domain internals.
	CategoryIntegration Category = "integration"
)

// ToIntegrationEvent converts a domain event into an integration event with
// the given payload. The envelope fields (aggregate info, correlation, etc.)
// are copied from the source event; the payload is replaced.
func ToIntegrationEvent(domainEvt Event, eventType string, payload any) Event {
	return NewEvent(
		eventType,
		payload,
		WithAggregateInfo(domainEvt.AggregateID(), domainEvt.AggregateType()),
		WithCorrelationID(domainEvt.GetMetadata().CorrelationID),
		WithCausationID(domainEvt.EventID()),
		WithTraceID(domainEvt.GetMetadata().TraceID),
		WithSource(domainEvt.GetMetadata().Source),
	)
}
