// transport_name.go — Stamp identifying the target transport for routing.
package stamp

// TransportNameStamp identifies the target transport for routing.
type TransportNameStamp struct {
	Name string
}

func (TransportNameStamp) StampName() string { return NameTransportName }
