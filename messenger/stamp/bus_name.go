// bus_name.go — Stamp identifying which bus instance dispatched the message.
package stamp

// BusNameStamp identifies which bus dispatched the message.
type BusNameStamp struct {
	Name string
}

func (BusNameStamp) StampName() string { return NameBusName }
