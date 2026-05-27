package runtime

import (
	"testing"
	"time"
)

func TestNewHTTP2DialConfig(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareRuntime(testSession(t, "http2"))
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	cfg, err := NewHTTP2DialConfig(prepared, "example.com:443", 3*time.Second)
	if err != nil {
		t.Fatalf("new http2 dial config: %v", err)
	}
	if cfg.Address != "example.com:443" {
		t.Fatalf("unexpected address: %s", cfg.Address)
	}
	if cfg.TLSConfig == nil {
		t.Fatal("expected tls config")
	}
	if cfg.TLSConfig.ServerName != edgeServerNameHTTP2 {
		t.Fatalf("unexpected server name: %s", cfg.TLSConfig.ServerName)
	}
	if cfg.Timeout != 3*time.Second {
		t.Fatalf("unexpected timeout: %s", cfg.Timeout)
	}
}

func TestNewHTTP2DialConfigRejectsMissingAddress(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareRuntime(testSession(t, "http2"))
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	if _, err := NewHTTP2DialConfig(prepared, "", 0); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewHTTP2DialConfigWithProvider(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareRuntime(testSession(t, "http2"))
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	cfg, err := NewHTTP2DialConfigWithProvider(
		prepared,
		"",
		StaticEdgeAddressProvider{Address: "198.51.100.10:443"},
		2*time.Second,
	)
	if err != nil {
		t.Fatalf("new http2 dial config with provider: %v", err)
	}
	if cfg.AddressProvider == nil {
		t.Fatal("expected address provider")
	}
	address, err := cfg.resolveAddress()
	if err != nil {
		t.Fatalf("resolve address: %v", err)
	}
	if address != "198.51.100.10:443" {
		t.Fatalf("unexpected resolved address: %s", address)
	}
}

func TestHTTP2DialConfigTransportFactory(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareRuntime(testSession(t, "http2"))
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}
	cfg, err := NewHTTP2DialConfig(prepared, "example.com:443", 0)
	if err != nil {
		t.Fatalf("new http2 dial config: %v", err)
	}

	factory, err := cfg.TransportFactory()
	if err != nil {
		t.Fatalf("transport factory: %v", err)
	}
	if factory == nil {
		t.Fatal("expected transport factory")
	}
}

func TestHTTP2DialConfigUsesAddressProvider(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareRuntime(testSession(t, "http2"))
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}
	cfg, err := NewHTTP2DialConfig(prepared, "", 0)
	if err == nil {
		t.Fatal("expected error without address or provider")
	}

	cfg = &HTTP2DialConfig{
		AddressProvider: StaticEdgeAddressProvider{Address: "198.51.100.10:443"},
		TLSConfig:       testTLSConfig(),
		Timeout:         time.Second,
	}
	factory, err := cfg.TransportFactory()
	if err != nil {
		t.Fatalf("transport factory: %v", err)
	}
	dialFactory, ok := factory.(DialHTTP2TransportFactory)
	if !ok {
		t.Fatalf("unexpected factory type: %T", factory)
	}
	if dialFactory.Address != "198.51.100.10:443" {
		t.Fatalf("unexpected resolved address: %s", dialFactory.Address)
	}
}
