// force.go — Stamps for overriding dispatch routing (force sync, force transport).
package stamp

// ForceSyncStamp overrides async routing, forcing synchronous dispatch.
type ForceSyncStamp struct{}

func (ForceSyncStamp) StampName() string { return NameForceSync }

// ForceTransportStamp overrides routing to target a specific transport.
type ForceTransportStamp struct {
	TransportName string
}

func (ForceTransportStamp) StampName() string { return NameForceTransport }
