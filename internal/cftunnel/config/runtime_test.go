package config

import (
	"testing"
	"time"

	appconfig "github.com/jians1/cf-tunnel/internal/config"
)

func TestNormalizeDefaultsQuickTunnelRuntime(t *testing.T) {
	t.Parallel()

	cfg, err := Normalize(appconfig.CFTunnelConfig{
		EdgeProtocol: appconfig.EdgeProtocolQUIC,
		Target:       "http://127.0.0.1:8080",
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.EdgeProtocol != appconfig.EdgeProtocolQUIC {
		t.Fatalf("unexpected edge protocol: %s", cfg.EdgeProtocol)
	}
	if cfg.HAConnections != 4 {
		t.Fatalf("unexpected ha connections: %d", cfg.HAConnections)
	}
	if cfg.Origin.URL.String() != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected origin url: %s", cfg.Origin.URL.String())
	}
}

func TestNormalizeCarriesRuntimeFields(t *testing.T) {
	t.Parallel()

	cfg, err := Normalize(appconfig.CFTunnelConfig{
		QuickService: "https://example.com",
		EdgeProtocol: appconfig.EdgeProtocolHTTP2,
		HAConnections: 2,
		Target:       "http://127.0.0.1:8080",
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.QuickService != "https://example.com" {
		t.Fatalf("unexpected quick service: %s", cfg.QuickService)
	}
	if cfg.HAConnections != 2 {
		t.Fatalf("unexpected ha connections: %d", cfg.HAConnections)
	}
	if cfg.QuickServiceTimeout != 15*time.Second {
		t.Fatalf("unexpected quick service timeout: %s", cfg.QuickServiceTimeout)
	}
	if len(cfg.RetryBackoffs) != 2 || cfg.RetryBackoffs[0] != 500*time.Millisecond || cfg.RetryBackoffs[1] != 1500*time.Millisecond {
		t.Fatalf("unexpected retry backoffs: %#v", cfg.RetryBackoffs)
	}
}
