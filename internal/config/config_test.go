package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRequiresTunnelTarget(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{"--health-listen="})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseAcceptsShutdownTimeout(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--shutdown-timeout=750ms",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ShutdownTimeout.String() != "750ms" {
		t.Fatalf("unexpected shutdown timeout: %s", cfg.ShutdownTimeout)
	}
}

func TestParseRejectsNonPositiveShutdownTimeout(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--shutdown-timeout=0s",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseRejectsHostPortTunnelTarget(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-tunnel-target=127.0.0.1:8080",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseRejectsRemovedOriginProtocolFlag(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-origin-protocol=http",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected removed flag to be rejected")
	}
}

func TestParseRejectsRemovedCFTunnelDebugFlags(t *testing.T) {
	t.Parallel()

	removed := []string{
		"--cf-mock-quick-service",
		"--cf-transport-validate-only",
		"--cf-transport-validate-run-selected",
		"--cf-transport-validate-timeout=1s",
		"--cf-http2-preflight-only",
		"--cf-http2-preflight-policy=fail-fast",
		"--cf-http2-edge-address=198.51.100.10:443",
		"--cf-http2-edge-discovery",
		"--cf-http2-edge-ip-version=6",
		"--cf-quic-preflight-only",
		"--cf-quic-preflight-policy=fail-fast",
		"--cf-quic-edge-address=198.51.100.10:7844",
		"--cf-quic-edge-discovery",
		"--cf-quic-edge-ip-version=6",
		"--cf-quick-service=https://example.com",
		"--cf-quick-service-timeout=3s",
		"--cf-quick-service-retry-backoff=10ms,20ms",
		"--cf-tunnel=name=t1,target=http://127.0.0.1:8081",
	}

	for _, flag := range removed {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()

			_, err := Parse([]string{
				"--cf-edge-protocol=http2",
				"--cf-tunnel-target=http://127.0.0.1:8080",
				flag,
				"--health-listen=",
			})
			if err == nil {
				t.Fatal("expected removed flag to be rejected")
			}
		})
	}
}

func TestParseAcceptsCFTunnelRuntimeFlags(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--cf-edge-protocol=http2",
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CFTunnel.QuickService != "https://api.trycloudflare.com" {
		t.Fatalf("unexpected quick service: %s", cfg.CFTunnel.QuickService)
	}
}

func TestParseTunnelTokenFromFlag(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-tunnel-token=token-from-flag",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.CFTunnel.TunnelToken != "token-from-flag" {
		t.Fatalf("unexpected token %q", cfg.CFTunnel.TunnelToken)
	}
}

func TestParseTunnelTokenFromEnv(t *testing.T) {
	t.Setenv("CF_TUNNEL_TOKEN", "token-from-env")

	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.CFTunnel.TunnelToken != "token-from-env" {
		t.Fatalf("unexpected token %q", cfg.CFTunnel.TunnelToken)
	}
}

func TestTunnelTokenFlagOverridesEnv(t *testing.T) {
	t.Setenv("CF_TUNNEL_TOKEN", "token-from-env")

	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-tunnel-token=token-from-flag",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.CFTunnel.TunnelToken != "token-from-flag" {
		t.Fatalf("expected flag token to win, got %q", cfg.CFTunnel.TunnelToken)
	}
}

func TestParseTunnelTokenTrimsFlagValue(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-tunnel-token= token-from-flag ",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.CFTunnel.TunnelToken != "token-from-flag" {
		t.Fatalf("unexpected token %q", cfg.CFTunnel.TunnelToken)
	}
}

func TestTunnelTokenStillRequiresTarget(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-tunnel-token=token-from-flag",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected missing target validation error")
	}
	if !strings.Contains(err.Error(), "cf-tunnel-target is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRejectsRemovedHAConnectionsFlag(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-edge-protocol=http2",
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-ha-connections=1",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected removed flag to be rejected")
	}
}

func TestParseRejectsAutoEdgeProtocol(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-edge-protocol=auto",
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseDefaultRoutesIsEmpty(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CFTunnel.Routes) != 0 {
		t.Fatalf("expected empty routes by default, got %d", len(cfg.CFTunnel.Routes))
	}
}

func TestParseDefaultEdgeProtocolIsHTTP2(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CFTunnel.EdgeProtocol != EdgeProtocolHTTP2 {
		t.Fatalf("expected default edge protocol %q, got %q", EdgeProtocolHTTP2, cfg.CFTunnel.EdgeProtocol)
	}
}

func TestCFTunnelValidateRouteRequiresPathAndTarget(t *testing.T) {
	t.Parallel()

	base := CFTunnelConfig{
		EdgeProtocol: EdgeProtocolHTTP2,
		Target:       "http://127.0.0.1:8080",
	}

	tests := []struct {
		name   string
		routes []RouteRule
	}{
		{
			name: "missing path",
			routes: []RouteRule{
				{Path: "", Target: "http://127.0.0.1:9001"},
			},
		},
		{
			name: "missing target",
			routes: []RouteRule{
				{Path: "/api/*", Target: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := base
			cfg.Routes = tt.routes
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCFTunnelValidateRoutePathGrammar(t *testing.T) {
	t.Parallel()

	base := CFTunnelConfig{
		EdgeProtocol: EdgeProtocolHTTP2,
		Target:       "http://127.0.0.1:8080",
	}

	valid := []RouteRule{
		{Path: "/", Target: "http://127.0.0.1:9000"},
		{Path: "/health", Target: "http://127.0.0.1:9001"},
		{Path: "/api/*", Target: "http://127.0.0.1:9002"},
	}
	cfg := base
	cfg.Routes = valid
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid route grammar, got: %v", err)
	}

	invalidPaths := []string{
		"*",
		"/api*",
		"/a*b",
		"api/*",
		"/api//",
	}
	for _, p := range invalidPaths {
		t.Run("invalid_"+p, func(t *testing.T) {
			t.Parallel()
			cfg := base
			cfg.Routes = []RouteRule{
				{Path: p, Target: "http://127.0.0.1:9001"},
			}
			if err := cfg.Validate(); err == nil {
				t.Fatalf("expected validation error for path %q", p)
			}
		})
	}
}

func TestCFTunnelValidateRouteRejectsDuplicateRules(t *testing.T) {
	t.Parallel()

	base := CFTunnelConfig{
		EdgeProtocol: EdgeProtocolHTTP2,
		Target:       "http://127.0.0.1:8080",
	}

	tests := []struct {
		name   string
		routes []RouteRule
	}{
		{
			name: "duplicate exact",
			routes: []RouteRule{
				{Path: "/health", Target: "http://127.0.0.1:9001"},
				{Path: "/health", Target: "http://127.0.0.1:9002"},
			},
		},
		{
			name: "duplicate prefix normalized",
			routes: []RouteRule{
				{Path: "/api/*", Target: "http://127.0.0.1:9001"},
				{Path: "/api//*", Target: "http://127.0.0.1:9002"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := base
			cfg.Routes = tt.routes
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected duplicate rule validation error")
			}
		})
	}
}

func TestParseAcceptsRepeatedCFRouteFlags(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--cf-edge-protocol=http2",
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-route=/api/*=http://127.0.0.1:9001",
		"--cf-route=/ws/*=ws://127.0.0.1:10000",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CFTunnel.Routes) != 2 {
		t.Fatalf("unexpected routes len: %d", len(cfg.CFTunnel.Routes))
	}
	if cfg.CFTunnel.Routes[0].Path != "/api/*" || cfg.CFTunnel.Routes[0].Target != "http://127.0.0.1:9001" {
		t.Fatalf("unexpected first route: %#v", cfg.CFTunnel.Routes[0])
	}
	if cfg.CFTunnel.Routes[1].Path != "/ws/*" || cfg.CFTunnel.Routes[1].Target != "ws://127.0.0.1:10000" {
		t.Fatalf("unexpected second route: %#v", cfg.CFTunnel.Routes[1])
	}
}

func TestParseAcceptsCFRouteTLSOptions(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--cf-edge-protocol=http2",
		"--cf-tunnel-target=https://127.0.0.1:8080",
		"--cf-origin-server-name=default.internal",
		"--cf-origin-insecure-skip-verify",
		"--cf-route=/api/*=https://127.0.0.1:9001,server_name=api.internal,insecure_skip_verify=true",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CFTunnel.Routes) != 1 {
		t.Fatalf("unexpected routes len: %d", len(cfg.CFTunnel.Routes))
	}
	route := cfg.CFTunnel.Routes[0]
	if route.Target != "https://127.0.0.1:9001" {
		t.Fatalf("unexpected route target: %s", route.Target)
	}
	if route.OriginServerName != "api.internal" {
		t.Fatalf("unexpected route server name: %s", route.OriginServerName)
	}
	if !route.InsecureSkipVerify {
		t.Fatal("expected route insecure skip verify")
	}
}

func TestParseRouteAcceptsHostOption(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-route=/api/*=http://127.0.0.1:9001,host=api.example.com",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if len(cfg.CFTunnel.Routes) != 1 {
		t.Fatalf("expected one route, got %d", len(cfg.CFTunnel.Routes))
	}
	if cfg.CFTunnel.Routes[0].Host != "api.example.com" {
		t.Fatalf("unexpected route host %q", cfg.CFTunnel.Routes[0].Host)
	}
}

func TestParseRouteRejectsInvalidHostOption(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--cf-route=/api/*=http://127.0.0.1:9001,host=bad host",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected invalid host error")
	}
	if !strings.Contains(err.Error(), "route[0].host") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRejectsInvalidCFRouteFormat(t *testing.T) {
	t.Parallel()

	invalid := []string{
		"--cf-route=",
		"--cf-route=/api/*",
		"--cf-route==127.0.0.1:9001",
		"--cf-route=/api/*=",
	}

	for _, flag := range invalid {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]string{
				"--cf-edge-protocol=http2",
				"--cf-tunnel-target=http://127.0.0.1:8080",
				flag,
				"--health-listen=",
			})
			if err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}

func TestParseYAMLConfigFileMultiTunnel(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "cfg.yaml")
	err := os.WriteFile(p, []byte(`
health_listen: ""
tunnels:
  - name: alpha
    cf_tunnel:
      quick_service: https://api.trycloudflare.com
      edge_protocol: http2
      target: http://127.0.0.1:8081
  - name: beta
    cf_tunnel:
      quick_service: https://api.trycloudflare.com
      edge_protocol: quic
      target: ws://127.0.0.1:10000
`), 0o644)
	if err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := Parse([]string{"--config=" + p})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Tunnels) != 2 {
		t.Fatalf("unexpected tunnel count: %d", len(cfg.Tunnels))
	}
	if cfg.Tunnels[0].Name != "alpha" || cfg.Tunnels[1].Name != "beta" {
		t.Fatalf("unexpected tunnel names: %#v", cfg.Tunnels)
	}
}

func TestParseYAMLConfigFileOverridesSingleTunnelCLI(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "cfg.yaml")
	err := os.WriteFile(p, []byte(`
cf_tunnel:
  quick_service: https://api.trycloudflare.com
  edge_protocol: http2
  target: http://127.0.0.1:9001
`), 0o644)
	if err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := Parse([]string{
		"--cf-edge-protocol=quic",
		"--cf-tunnel-target=http://127.0.0.1:8080",
		"--config=" + p,
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CFTunnel.EdgeProtocol != "http2" {
		t.Fatalf("expected config file edge protocol override, got: %s", cfg.CFTunnel.EdgeProtocol)
	}
	if cfg.CFTunnel.Target != "http://127.0.0.1:9001" {
		t.Fatalf("expected config file target override, got: %s", cfg.CFTunnel.Target)
	}
}

func TestParseYAMLConfigFileParsesFormalTunnelRoutes(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "cfg.yaml")
	err := os.WriteFile(p, []byte(`
health_listen: ""
cf_tunnel:
  tunnel_token: token-from-yaml
  edge_protocol: quic
  target: http://127.0.0.1:13000
  routes:
    - host: test.910666.xyz
      path: /api/*
      target: http://127.0.0.1:13000
      origin_server_name: api.internal
      insecure_skip_verify: true
`), 0o644)
	if err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := Parse([]string{"--config=" + p})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CFTunnel.TunnelToken != "token-from-yaml" {
		t.Fatalf("unexpected tunnel token: %q", cfg.CFTunnel.TunnelToken)
	}
	if len(cfg.CFTunnel.Routes) != 1 {
		t.Fatalf("unexpected routes len: %d", len(cfg.CFTunnel.Routes))
	}
	route := cfg.CFTunnel.Routes[0]
	if route.Host != "test.910666.xyz" || route.Path != "/api/*" || route.Target != "http://127.0.0.1:13000" {
		t.Fatalf("unexpected route: %#v", route)
	}
	if route.OriginServerName != "api.internal" || !route.InsecureSkipVerify {
		t.Fatalf("unexpected route TLS settings: %#v", route)
	}
	if !route.InsecureSkipVerifySet {
		t.Fatal("expected route insecure skip verify to be marked as explicitly set")
	}
}

func TestParseYAMLConfigRejectsInternalRouteFields(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "cfg.yaml")
	err := os.WriteFile(p, []byte(`
health_listen: ""
cf_tunnel:
  edge_protocol: quic
  target: http://127.0.0.1:13000
  routes:
    - path: /api/*
      target: http://127.0.0.1:13000
      insecure_skip_verify_set: true
`), 0o644)
	if err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, err = Parse([]string{"--config=" + p})
	if err == nil {
		t.Fatal("expected internal route field rejection")
	}
	if !strings.Contains(err.Error(), "field insecure_skip_verify_set not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseYAMLConfigRejectsJSONFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "cfg.json")
	err := os.WriteFile(p, []byte(`{"health_listen":"","cf_tunnel":{"edge_protocol":"http2","target":"http://127.0.0.1:9001"}}`), 0o644)
	if err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, err = Parse([]string{"--config=" + p})
	if err == nil {
		t.Fatal("expected JSON config rejection")
	}
	if !strings.Contains(err.Error(), "YAML") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseYAMLConfigRejectsGoStyleFields(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "cfg.yaml")
	err := os.WriteFile(p, []byte(`
health_listen: ""
tunnels:
  - name: alpha
    CFTunnel:
      EdgeProtocol: http2
      Target: http://127.0.0.1:8081
`), 0o644)
	if err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, err = Parse([]string{"--config=" + p})
	if err == nil {
		t.Fatal("expected Go-style field rejection")
	}
	if !strings.Contains(err.Error(), "field CFTunnel not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseYAMLConfigFileRejectsDuplicateTunnelName(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "cfg.yaml")
	err := os.WriteFile(p, []byte(`
tunnels:
  - name: dup
    cf_tunnel:
      quick_service: https://api.trycloudflare.com
      edge_protocol: http2
      target: http://127.0.0.1:8081
  - name: dup
    cf_tunnel:
      quick_service: https://api.trycloudflare.com
      edge_protocol: http2
      target: http://127.0.0.1:8082
`), 0o644)
	if err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, err = Parse([]string{"--config=" + p})
	if err == nil {
		t.Fatal("expected duplicate tunnel name validation error")
	}
}
