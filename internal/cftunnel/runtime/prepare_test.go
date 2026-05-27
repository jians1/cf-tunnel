package runtime

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/origin"
)

func TestPrepareRuntimeForQUIC(t *testing.T) {
	t.Parallel()

	session := testSession(t, "quic")
	prepared, err := PrepareRuntime(session)
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}
	if prepared.OriginProxy == nil {
		t.Fatal("expected origin proxy")
	}
	if _, ok := prepared.EdgeTLSByProto["quic"]; !ok {
		t.Fatal("expected quic tls config")
	}
	cfg := prepared.EdgeTLSByProto["quic"]
	if cfg.ServerName != edgeServerNameQUIC {
		t.Fatalf("unexpected server name: %s", cfg.ServerName)
	}
	if len(cfg.NextProtos) != 1 || cfg.NextProtos[0] != edgeALPNQUIC {
		t.Fatalf("unexpected alpn: %v", cfg.NextProtos)
	}
}

func TestPrepareRuntimeForHTTP2(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	prepared, err := PrepareRuntime(session)
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}
	cfg := prepared.EdgeTLSByProto["http2"]
	if cfg.ServerName != edgeServerNameHTTP2 {
		t.Fatalf("unexpected server name: %s", cfg.ServerName)
	}
	if len(cfg.NextProtos) != 0 {
		t.Fatalf("unexpected alpn: %v", cfg.NextProtos)
	}
}

func TestPrepareRuntimeRejectsUnsupportedProtocol(t *testing.T) {
	t.Parallel()

	session := testSession(t, "bogus")
	if _, err := PrepareRuntime(session); err == nil {
		t.Fatal("expected error")
	}
}

func TestOriginTargetRebuild(t *testing.T) {
	t.Parallel()

	session := testSession(t, "quic")
	target, err := session.OriginTarget()
	if err != nil {
		t.Fatalf("origin target: %v", err)
	}
	if target.URL.String() != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected target url: %s", target.URL.String())
	}
}

func testSession(t *testing.T, proto string) Session {
	t.Helper()

	return Session{
		TunnelID:   uuid.MustParse("11111111-1111-1111-1111-111111111111").String(),
		AccountTag: "acct",
		Secret:     []byte("secret"),
		Hostname:   "demo.trycloudflare.com",
		PublicURL:  "https://demo.trycloudflare.com",
		Edge: EdgeSettings{
			Protocol: proto,
		},
		Origin: OriginSettings{
			RawTarget:            "127.0.0.1:8080",
			URL:                  "http://127.0.0.1:8080",
			Protocol:             origin.ProtocolHTTP,
			ServerName:           "origin.example.com",
			InsecureSkipVerify:   true,
			WebsocketUpgradeMode: false,
		},
		QuickTunnel:   true,
		HAConnections: 1,
	}
}

var (
	_ http.Handler = http.Handler(nil)
	_ tls.CurveID  = tls.CurveP256
)
