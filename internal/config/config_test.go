package config

import "testing"

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
		"--cf-tunnel-target=127.0.0.1:8080",
		"--cf-origin-protocol=http",
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
		"--cf-tunnel-target=127.0.0.1:8080",
		"--cf-origin-protocol=http",
		"--shutdown-timeout=0s",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseRequiresOriginProtocolForHostPortTarget(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-tunnel-target=127.0.0.1:8080",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseAcceptsHostPortTargetWithExplicitOriginProtocol(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--cf-tunnel-target=127.0.0.1:8080",
		"--cf-origin-protocol=http",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CFTunnel.OriginProtocol != ProtocolHTTP {
		t.Fatalf("unexpected origin protocol: %s", cfg.CFTunnel.OriginProtocol)
	}
}

func TestParseRejectsIncompatibleOriginOverride(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-tunnel-target=https://127.0.0.1:8443",
		"--cf-origin-protocol=http",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected validation error")
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
		"--cf-quick-service-timeout=3s",
		"--cf-quick-service-retry-backoff=10ms,20ms",
	}

	for _, flag := range removed {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()

			_, err := Parse([]string{
				"--cf-edge-protocol=http2",
				"--cf-tunnel-target=127.0.0.1:8080",
				"--cf-origin-protocol=http",
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
		"--cf-quick-service=https://example.com",
		"--cf-edge-protocol=http2",
		"--cf-tunnel-target=127.0.0.1:8080",
		"--cf-origin-protocol=http",
		"--cf-ha-connections=1",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CFTunnel.QuickService != "https://example.com" {
		t.Fatalf("unexpected quick service: %s", cfg.CFTunnel.QuickService)
	}
	if cfg.CFTunnel.HAConnections != 1 {
		t.Fatalf("unexpected ha connections: %d", cfg.CFTunnel.HAConnections)
	}
}

func TestParseRejectsQuickTunnelHAConnectionsAboveOne(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-edge-protocol=http2",
		"--cf-tunnel-target=127.0.0.1:8080",
		"--cf-origin-protocol=http",
		"--cf-ha-connections=2",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseRejectsAutoEdgeProtocol(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--cf-edge-protocol=auto",
		"--cf-tunnel-target=127.0.0.1:8080",
		"--cf-origin-protocol=http",
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
		"--cf-tunnel-target=127.0.0.1:8080",
		"--cf-origin-protocol=http",
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
		EdgeProtocol:   EdgeProtocolHTTP2,
		HAConnections:  1,
		Target:         "127.0.0.1:8080",
		OriginProtocol: ProtocolHTTP,
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
		EdgeProtocol:   EdgeProtocolHTTP2,
		HAConnections:  1,
		Target:         "127.0.0.1:8080",
		OriginProtocol: ProtocolHTTP,
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
		EdgeProtocol:   EdgeProtocolHTTP2,
		HAConnections:  1,
		Target:         "127.0.0.1:8080",
		OriginProtocol: ProtocolHTTP,
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
		"--cf-tunnel-target=127.0.0.1:8080",
		"--cf-origin-protocol=http",
		"--cf-route=/api/*=127.0.0.1:9001",
		"--cf-route=/ws/*=ws://127.0.0.1:10000",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CFTunnel.Routes) != 2 {
		t.Fatalf("unexpected routes len: %d", len(cfg.CFTunnel.Routes))
	}
	if cfg.CFTunnel.Routes[0].Path != "/api/*" || cfg.CFTunnel.Routes[0].Target != "127.0.0.1:9001" {
		t.Fatalf("unexpected first route: %#v", cfg.CFTunnel.Routes[0])
	}
	if cfg.CFTunnel.Routes[1].Path != "/ws/*" || cfg.CFTunnel.Routes[1].Target != "ws://127.0.0.1:10000" {
		t.Fatalf("unexpected second route: %#v", cfg.CFTunnel.Routes[1])
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
				"--cf-tunnel-target=127.0.0.1:8080",
				"--cf-origin-protocol=http",
				flag,
				"--health-listen=",
			})
			if err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}
