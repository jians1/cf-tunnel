package origin

import (
	"fmt"
	"net/url"
)

type Protocol string

const (
	ProtocolHTTP  Protocol = "http"
	ProtocolHTTPS Protocol = "https"
	ProtocolWS    Protocol = "ws"
	ProtocolWSS   Protocol = "wss"
)

type Target struct {
	Raw                  string
	Protocol             Protocol
	URL                  *url.URL
	ServerName           string
	InsecureSkipVerify   bool
	WebsocketUpgradeMode bool
}

func ParseTarget(rawTarget, serverName string, insecureSkipVerify bool) (Target, error) {
	if rawTarget == "" {
		return Target{}, fmt.Errorf("empty target")
	}

	u, err := url.Parse(rawTarget)
	if err != nil {
		return Target{}, fmt.Errorf("parse target url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return Target{}, fmt.Errorf("target must be a full URL with scheme and host")
	}

	proto, err := resolveProtocol(u.Scheme)
	if err != nil {
		return Target{}, err
	}
	return Target{
		Raw:                  rawTarget,
		Protocol:             proto,
		URL:                  normalizeURL(u, proto),
		ServerName:           serverName,
		InsecureSkipVerify:   insecureSkipVerify,
		WebsocketUpgradeMode: proto == ProtocolWS || proto == ProtocolWSS,
	}, nil
}

func resolveProtocol(v string) (Protocol, error) {
	switch v {
	case "http":
		return ProtocolHTTP, nil
	case "https":
		return ProtocolHTTPS, nil
	case "ws":
		return ProtocolWS, nil
	case "wss":
		return ProtocolWSS, nil
	default:
		return "", fmt.Errorf("unsupported target scheme: %s", v)
	}
}

func normalizeURL(u *url.URL, proto Protocol) *url.URL {
	out := *u
	out.Scheme = schemeForProtocol(proto)
	return &out
}

func schemeForProtocol(proto Protocol) string {
	switch proto {
	case ProtocolWS:
		return "http"
	case ProtocolWSS:
		return "https"
	default:
		return string(proto)
	}
}
