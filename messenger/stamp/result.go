// result.go — Stamp holding the handler's return value.
package stamp

// ResultStamp holds the handler return value.
type ResultStamp struct {
	Value any
}

func (ResultStamp) StampName() string { return NameResult }
