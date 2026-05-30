package runtime

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"sync"

	"github.com/jians1/cf-tunnel/internal/cftunnel/origin"
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

var (
	edgeRootCAPoolOnce sync.Once
	edgeRootCAPool     *x509.CertPool
	edgeRootCAPoolErr  error
)

func PrepareRuntime(session Session) (*PreparedRuntime, error) {
	originTarget, err := session.OriginTarget()
	if err != nil {
		return nil, fmt.Errorf("rebuild origin target: %w", err)
	}

	proxy, err := origin.NewRoutedProxy(originTarget, session.Origin.Routes)
	if err != nil {
		return nil, fmt.Errorf("build routed origin proxy: %w", err)
	}
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
	protocol, err := ParseEdgeProtocol(selected)
	if err != nil {
		return nil, err
	}

	switch protocol {
	case EdgeProtocol(edgeProtocolQUIC):
		cfg, err := newEdgeTLSConfig(edgeServerNameQUIC, []string{edgeALPNQUIC})
		if err != nil {
			return nil, err
		}
		return map[string]*tls.Config{"quic": cfg}, nil
	case EdgeProtocol(edgeProtocolHTTP2):
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
	rootCAs, err := edgeTLSRootCAPool()
	if err != nil {
		return nil, err
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

func edgeTLSRootCAPool() (*x509.CertPool, error) {
	edgeRootCAPoolOnce.Do(func() {
		pool, err := x509.SystemCertPool()
		if err != nil {
			edgeRootCAPoolErr = fmt.Errorf("load system root CAs: %w", err)
			return
		}
		if !pool.AppendCertsFromPEM([]byte(cloudflareRootCAPEM)) {
			edgeRootCAPoolErr = fmt.Errorf("load cloudflare root CAs")
			return
		}
		edgeRootCAPool = pool
	})
	if edgeRootCAPoolErr != nil {
		return nil, edgeRootCAPoolErr
	}
	return edgeRootCAPool, nil
}
