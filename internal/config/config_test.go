package config

import "testing"

func TestParseRejectsNoEnabledFeatures(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{"--health-listen="})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseAcceptsShutdownTimeout(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--enable-cf-tunnel",
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
		"--enable-cf-tunnel",
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
		"--enable-cf-tunnel",
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
		"--enable-cf-tunnel",
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

func TestParseRejectsIPv6PoolWithoutSource(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--enable-ipv6-pool",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseAcceptsIPv6PoolWithCIDR(t *testing.T) {
	t.Parallel()

	cfg, err := Parse([]string{
		"--enable-ipv6-pool",
		"--ipv6-pool-cidr=2001:db8::/120",
		"--health-listen=",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IPv6Pool.Strategy != IPv6StrategyRandom {
		t.Fatalf("unexpected strategy: %s", cfg.IPv6Pool.Strategy)
	}
}

func TestParseRejectsIncompatibleOriginOverride(t *testing.T) {
	t.Parallel()

	_, err := Parse([]string{
		"--enable-cf-tunnel",
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
				"--enable-cf-tunnel",
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
		"--enable-cf-tunnel",
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
		"--enable-cf-tunnel",
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
		"--enable-cf-tunnel",
		"--cf-edge-protocol=auto",
		"--cf-tunnel-target=127.0.0.1:8080",
		"--cf-origin-protocol=http",
		"--health-listen=",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
