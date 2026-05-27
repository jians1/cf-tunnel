package runtime

import (
	"errors"
	"testing"
	"time"
)

func TestNewQUICDialConfig(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareRuntime(testSession(t, "quic"))
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	cfg, err := NewQUICDialConfig(prepared, "example.com:7844", 3*time.Second)
	if err != nil {
		t.Fatalf("new quic dial config: %v", err)
	}
	if cfg.Address != "example.com:7844" {
		t.Fatalf("unexpected address: %s", cfg.Address)
	}
	if cfg.TLSConfig == nil {
		t.Fatal("expected tls config")
	}
	if cfg.TLSConfig.ServerName != edgeServerNameQUIC {
		t.Fatalf("unexpected server name: %s", cfg.TLSConfig.ServerName)
	}
	if len(cfg.TLSConfig.NextProtos) != 1 || cfg.TLSConfig.NextProtos[0] != edgeALPNQUIC {
		t.Fatalf("unexpected next protos: %v", cfg.TLSConfig.NextProtos)
	}
}

func TestQUICDialConfigWithProvider(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareRuntime(testSession(t, "quic"))
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	cfg, err := NewQUICDialConfigWithProvider(prepared, "", StaticEdgeAddressProvider{
		Address:     "198.51.100.20:7844",
		QUICAddress: "198.51.100.20:7844",
	}, time.Second)
	if err != nil {
		t.Fatalf("new quic dial config with provider: %v", err)
	}
	address, err := cfg.resolveAddress()
	if err != nil {
		t.Fatalf("resolve address: %v", err)
	}
	if address != "198.51.100.20:7844" {
		t.Fatalf("unexpected resolved address: %s", address)
	}
}

func TestGenerateQUICServerTLSConfigRejectsNilEntropyReader(t *testing.T) {
	t.Parallel()

	_, err := generateQUICServerTLSConfigWithReader(nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errNilEntropyReader) {
		t.Fatalf("expected nil entropy reader error, got: %v", err)
	}
}
