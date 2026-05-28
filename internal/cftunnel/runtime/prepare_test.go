package runtime

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/origin"
	appconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
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
	assertEdgeTLSDefaults(t, cfg)
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
	assertEdgeTLSDefaults(t, cfg)
}

func TestPrepareRuntimeReusesEdgeRootCAPool(t *testing.T) {
	t.Parallel()

	first, err := PrepareRuntime(testSession(t, "http2"))
	if err != nil {
		t.Fatalf("prepare first runtime: %v", err)
	}
	second, err := PrepareRuntime(testSession(t, "http2"))
	if err != nil {
		t.Fatalf("prepare second runtime: %v", err)
	}

	if first.EdgeTLSByProto["http2"].RootCAs != second.EdgeTLSByProto["http2"].RootCAs {
		t.Fatal("expected prepared runtimes to reuse cached root CA pool")
	}
}

func assertEdgeTLSDefaults(t *testing.T, cfg *tls.Config) {
	t.Helper()

	if cfg.RootCAs == nil {
		t.Fatal("expected root CAs")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected min tls version: %x", cfg.MinVersion)
	}
	if len(cfg.CurvePreferences) != 1 || cfg.CurvePreferences[0] != tls.CurveP256 {
		t.Fatalf("unexpected curve preferences: %v", cfg.CurvePreferences)
	}
}

func TestCloudflareRootCAPEMIsParseable(t *testing.T) {
	t.Parallel()

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(cloudflareRootCAPEM)) {
		t.Fatal("expected cloudflare root CA PEM to parse")
	}
}

func TestPrepareRuntimeRejectsUnsupportedProtocol(t *testing.T) {
	t.Parallel()

	for _, proto := range []string{"bogus", "auto"} {
		session := testSession(t, proto)
		if _, err := PrepareRuntime(session); err == nil {
			t.Fatalf("expected error for protocol %q", proto)
		}
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

func TestPrepareRuntimeRoutesByPath(t *testing.T) {
	t.Parallel()

	defaultSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("default"))
	}))
	defer defaultSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("api"))
	}))
	defer apiSrv.Close()

	session := testSession(t, "http2")
	session.Origin.URL = defaultSrv.URL
	session.Origin.Routes = []appconfig.RouteRule{
		{Path: "/api/*", Target: apiSrv.URL},
	}

	prepared, err := PrepareRuntime(session)
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.test/api/ping", nil)
	rec := httptest.NewRecorder()
	prepared.OriginProxy.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Result().Body)
	_ = rec.Result().Body.Close()
	if string(body) != "api" {
		t.Fatalf("unexpected route body: %q", string(body))
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.test/other", nil)
	rec = httptest.NewRecorder()
	prepared.OriginProxy.ServeHTTP(rec, req)
	body, _ = io.ReadAll(rec.Result().Body)
	_ = rec.Result().Body.Close()
	if string(body) != "default" {
		t.Fatalf("unexpected default body: %q", string(body))
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
