package runtime

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/origin"
)

const (
	edgeServerNameHTTP2 = "h2.cftunnel.com"
	edgeServerNameQUIC  = "quic.cftunnel.com"
	edgeALPNQUIC        = "argotunnel"
)

type PreparedRuntime struct {
	Session        Session
	OriginProxy    http.Handler
	EdgeTLSByProto map[string]*tls.Config
}

func PrepareRuntime(session Session) (*PreparedRuntime, error) {
	originTarget, err := session.OriginTarget()
	if err != nil {
		return nil, fmt.Errorf("rebuild origin target: %w", err)
	}

	proxy := origin.NewProxy(originTarget)
	edgeTLS, err := buildEdgeTLSConfigs(session.Edge.Protocol)
	if err != nil {
		return nil, err
	}

	return &PreparedRuntime{
		Session:        session,
		OriginProxy:    proxy,
		EdgeTLSByProto: edgeTLS,
	}, nil
}

func buildEdgeTLSConfigs(selected string) (map[string]*tls.Config, error) {
	switch selected {
	case "quic":
		cfg, err := newEdgeTLSConfig(edgeServerNameQUIC, []string{edgeALPNQUIC})
		if err != nil {
			return nil, err
		}
		return map[string]*tls.Config{"quic": cfg}, nil
	case "http2":
		cfg, err := newEdgeTLSConfig(edgeServerNameHTTP2, nil)
		if err != nil {
			return nil, err
		}
		return map[string]*tls.Config{"http2": cfg}, nil
	default:
		return nil, fmt.Errorf("unsupported edge protocol: %s", selected)
	}
}

func newEdgeTLSConfig(serverName string, nextProtos []string) (*tls.Config, error) {
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system root CAs: %w", err)
	}
	if !rootCAs.AppendCertsFromPEM([]byte(cloudflareRootCAPEM)) {
		return nil, fmt.Errorf("load cloudflare root CAs")
	}

	cfg := &tls.Config{
		ServerName:       serverName,
		RootCAs:          rootCAs,
		MinVersion:       tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{tls.CurveP256},
	}
	if len(nextProtos) > 0 {
		cfg.NextProtos = append([]string(nil), nextProtos...)
	}
	return cfg, nil
}
