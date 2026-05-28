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

func ParseEdgeProtocol(raw string) (EdgeProtocol, error) {
	switch raw {
	case edgeProtocolHTTP2, edgeProtocolQUIC:
		return EdgeProtocol(raw), nil
	default:
		return "", fmt.Errorf("unsupported edge protocol: %s", raw)
	}
}

type staticProtocolSelector struct {
	current EdgeProtocol
}

func NewStaticProtocolSelector(protocol string) (ProtocolSelector, error) {
	current, err := ParseEdgeProtocol(protocol)
	if err != nil {
		return nil, err
	}
	return staticProtocolSelector{current: current}, nil
}

func (s staticProtocolSelector) Current() EdgeProtocol {
	return s.current
}

func (s staticProtocolSelector) Fallback() (EdgeProtocol, bool) {
	return s.current, false
}
