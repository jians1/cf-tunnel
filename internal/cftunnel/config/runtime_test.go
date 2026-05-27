package config

import (
	"testing"

	appconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

func TestNormalizeDefaultsQuickTunnelRuntime(t *testing.T) {
	t.Parallel()

	cfg, err := Normalize(appconfig.CFTunnelConfig{
		Enabled:        true,
		EdgeProtocol:   appconfig.EdgeProtocolAuto,
		Target:         "127.0.0.1:8080",
		OriginProtocol: appconfig.ProtocolHTTP,
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.EdgeProtocol != appconfig.EdgeProtocolQUIC {
		t.Fatalf("unexpected edge protocol: %s", cfg.EdgeProtocol)
	}
	if cfg.HAConnections != 1 {
		t.Fatalf("unexpected ha connections: %d", cfg.HAConnections)
	}
	if cfg.Origin.URL.String() != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected origin url: %s", cfg.Origin.URL.String())
	}
}

func TestNormalizeCarriesRuntimeFields(t *testing.T) {
	t.Parallel()

	cfg, err := Normalize(appconfig.CFTunnelConfig{
		Enabled:        true,
		QuickService:   "https://example.com",
		EdgeProtocol:   appconfig.EdgeProtocolHTTP2,
		Target:         "127.0.0.1:8080",
		OriginProtocol: appconfig.ProtocolHTTP,
		HAConnections:  1,
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if cfg.QuickService != "https://example.com" {
		t.Fatalf("unexpected quick service: %s", cfg.QuickService)
	}
	if cfg.HAConnections != 1 {
		t.Fatalf("unexpected ha connections: %d", cfg.HAConnections)
	}
}
