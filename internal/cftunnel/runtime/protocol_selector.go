package runtime

import "fmt"

const (
	edgeProtocolHTTP2 = "http2"
	edgeProtocolQUIC  = "quic"
)

type ProtocolSelector interface {
	Current() EdgeProtocol
	Fallback() (EdgeProtocol, bool)
}

type EdgeProtocol string

func (p EdgeProtocol) String() string {
	return string(p)
}

type staticProtocolSelector struct {
	current EdgeProtocol
}

func NewStaticProtocolSelector(protocol string) (ProtocolSelector, error) {
	switch protocol {
	case edgeProtocolHTTP2, edgeProtocolQUIC:
		return staticProtocolSelector{current: EdgeProtocol(protocol)}, nil
	default:
		return nil, fmt.Errorf("unsupported edge protocol: %s", protocol)
	}
}

func (s staticProtocolSelector) Current() EdgeProtocol {
	return s.current
}

func (s staticProtocolSelector) Fallback() (EdgeProtocol, bool) {
	return s.current, false
}
