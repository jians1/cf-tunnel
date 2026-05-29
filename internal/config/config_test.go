package config

import (
	"os"
	"path/filepath"
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
		"--cf-edge-protocol=http2",
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

func TestParseConfigFileMultiTunnel(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "cfg.json")
	err := os.WriteFile(p, []byte(`{
		"health_listen": "",
		"tunnels": [
			{
				"name": "alpha",
				"CFTunnel": {
					"QuickService": "https://api.trycloudflare.com",
					"EdgeProtocol": "http2",
					"Target": "http://127.0.0.1:8081"
				}
			},
			{
				"name": "beta",
				"CFTunnel": {
					"QuickService": "https://api.trycloudflare.com",
					"EdgeProtocol": "quic",
					"Target": "ws://127.0.0.1:10000"
				}
			}
		]
	}`), 0o644)
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

func TestParseConfigFileOverridesSingleTunnelCLI(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "cfg.json")
	err := os.WriteFile(p, []byte(`{
		"cf_tunnel": {
			"QuickService": "https://api.trycloudflare.com",
			"EdgeProtocol": "http2",
			"Target": "http://127.0.0.1:9001"
		}
	}`), 0o644)
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

func TestParseConfigFileRejectsDuplicateTunnelName(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	p := filepath.Join(tmp, "cfg.json")
	err := os.WriteFile(p, []byte(`{
		"tunnels": [
			{
				"name": "dup",
				"CFTunnel": {
					"QuickService": "https://api.trycloudflare.com",
					"EdgeProtocol": "http2",
					"Target": "http://127.0.0.1:8081"
				}
			},
			{
				"name": "dup",
				"CFTunnel": {
					"QuickService": "https://api.trycloudflare.com",
					"EdgeProtocol": "http2",
					"Target": "http://127.0.0.1:8082"
				}
			}
		]
	}`), 0o644)
	if err != nil {
		t.Fatalf("write config file: %v", err)
	}

	_, err = Parse([]string{"--config=" + p})
	if err == nil {
		t.Fatal("expected duplicate tunnel name validation error")
	}
}
