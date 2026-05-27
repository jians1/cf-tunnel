package origin

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	appconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
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

func ParseTarget(rawTarget, originProtocol, serverName string, insecureSkipVerify bool) (Target, error) {
	if rawTarget == "" {
		return Target{}, fmt.Errorf("empty target")
	}

	if strings.Contains(rawTarget, "://") {
		u, err := url.Parse(rawTarget)
		if err != nil {
			return Target{}, fmt.Errorf("parse target url: %w", err)
		}
		if u.Host == "" {
			return Target{}, fmt.Errorf("target url must include host")
		}

		proto, err := resolveURLProtocol(u.Scheme, originProtocol)
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

	host, port, err := net.SplitHostPort(rawTarget)
	if err != nil || host == "" || port == "" {
		return Target{}, fmt.Errorf("target must be a full URL or host:port")
	}

	proto, err := resolveProtocol(originProtocol)
	if err != nil {
		return Target{}, err
	}
	u := &url.URL{
		Scheme: schemeForProtocol(proto),
		Host:   net.JoinHostPort(host, port),
	}
	return Target{
		Raw:                  rawTarget,
		Protocol:             proto,
		URL:                  u,
		ServerName:           serverName,
		InsecureSkipVerify:   insecureSkipVerify,
		WebsocketUpgradeMode: proto == ProtocolWS || proto == ProtocolWSS,
	}, nil
}

func resolveURLProtocol(targetScheme, originProtocol string) (Protocol, error) {
	if originProtocol != "" && originProtocol != appconfig.ProtocolAuto {
		return resolveProtocol(originProtocol)
	}
	return resolveProtocol(targetScheme)
}

func resolveProtocol(v string) (Protocol, error) {
	switch v {
	case appconfig.ProtocolHTTP:
		return ProtocolHTTP, nil
	case appconfig.ProtocolHTTPS:
		return ProtocolHTTPS, nil
	case appconfig.ProtocolWS:
		return ProtocolWS, nil
	case appconfig.ProtocolWSS:
		return ProtocolWSS, nil
	default:
		return "", fmt.Errorf("unsupported origin protocol: %s", v)
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
