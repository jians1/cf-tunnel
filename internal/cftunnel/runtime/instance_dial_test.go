package runtime

import (
	"strings"
	"testing"
)

func TestNewInstanceWithDialAddress(t *testing.T) {
	t.Parallel()

	instance, err := NewInstanceWithOptions(testSession(t, "http2"), nil, HTTP2ServerOptions{
		DialAddress:      "198.51.100.10:443",
		TransportFactory: PipeHTTP2TransportFactory{},
	})
	if err != nil {
		t.Fatalf("new instance with dial address: %v", err)
	}
	if instance.HTTP2DialConfig == nil {
		t.Fatal("expected http2 dial config")
	}
	if instance.HTTP2DialConfig.Address != "198.51.100.10:443" {
		t.Fatalf("unexpected dial address: %s", instance.HTTP2DialConfig.Address)
	}
	if instance.HTTP2Server == nil {
		t.Fatal("expected http2 server")
	}
}

func TestNewInstanceWithEdgeAddressProvider(t *testing.T) {
	t.Parallel()

	instance, err := NewInstanceWithOptions(testSession(t, "http2"), nil, HTTP2ServerOptions{
		EdgeAddressProvider: StaticEdgeAddressProvider{Address: "198.51.100.20:443"},
		TransportFactory:    PipeHTTP2TransportFactory{},
	})
	if err != nil {
		t.Fatalf("new instance with edge address provider: %v", err)
	}
	if instance.HTTP2DialConfig == nil {
		t.Fatal("expected http2 dial config")
	}
	address, err := instance.HTTP2DialConfig.resolveAddress()
	if err != nil {
		t.Fatalf("resolve address: %v", err)
	}
	if address != "198.51.100.20:443" {
		t.Fatalf("unexpected resolved address: %s", address)
	}
	if instance.HTTP2Server == nil {
		t.Fatal("expected http2 server")
	}
}

func TestNewInstanceQUICRuntimeUsesDialConfig(t *testing.T) {
	t.Parallel()

	_, err := NewInstanceWithRuntimeOptions(testSession(t, "quic"), nil, InstanceOptions{
		QUIC: QUICRuntimeOptions{
			DialAddress: "not-a-udp-address",
		},
	})
	if err == nil {
		t.Fatal("expected quic dial config error")
	}
	if !strings.Contains(err.Error(), "parse quic dial address") {
		t.Fatalf("unexpected error: %v", err)
	}
}
