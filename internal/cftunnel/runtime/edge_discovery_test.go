package runtime

import (
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
)

func TestCloudflareEdgeAddressProviderResolveHTTP2Address(t *testing.T) {
	t.Parallel()

	restoreSRV := stubEdgeLookupSRV([]*net.SRV{
		{Target: "region1.example.com.", Port: 443},
		{Target: "region2.example.com.", Port: 7844},
	})
	defer restoreSRV()

	restoreIP := stubEdgeLookupIP(map[string][]net.IP{
		"region1.example.com.": {
			net.ParseIP("2001:db8::10"),
			net.ParseIP("198.51.100.10"),
		},
		"region2.example.com.": {
			net.ParseIP("198.51.100.20"),
		},
	})
	defer restoreIP()

	provider := NewCloudflareEdgeAddressProvider("", EdgeIPAuto, testEdgeLogger())
	addr, err := provider.ResolveHTTP2Address()
	if err != nil {
		t.Fatalf("resolve http2 address: %v", err)
	}
	if !strings.HasSuffix(addr, ":443") {
		t.Fatalf("unexpected port in address: %s", addr)
	}
}

func TestCloudflareEdgeAddressProviderUsesConnectionIndex(t *testing.T) {
	restoreSRV := stubEdgeLookupSRV([]*net.SRV{
		{Target: "region1.example.com.", Port: 443},
		{Target: "region2.example.com.", Port: 443},
	})
	defer restoreSRV()

	restoreIP := stubEdgeLookupIP(map[string][]net.IP{
		"region1.example.com.": {
			net.ParseIP("198.51.100.10"),
		},
		"region2.example.com.": {
			net.ParseIP("198.51.100.20"),
		},
	})
	defer restoreIP()

	provider := NewCloudflareEdgeAddressProvider("", EdgeIPv4Only, testEdgeLogger())

	first, err := provider.ForConnIndex(0).ResolveHTTP2Address()
	if err != nil {
		t.Fatalf("resolve first http2 address: %v", err)
	}
	second, err := provider.ForConnIndex(1).ResolveHTTP2Address()
	if err != nil {
		t.Fatalf("resolve second http2 address: %v", err)
	}

	if first != "198.51.100.10:443" {
		t.Fatalf("unexpected first address: %s", first)
	}
	if second != "198.51.100.20:443" {
		t.Fatalf("unexpected second address: %s", second)
	}
}

func TestCloudflareEdgeAddressProviderResolveQUICAddressHonorsIPv4Only(t *testing.T) {
	t.Parallel()

	restoreSRV := stubEdgeLookupSRV([]*net.SRV{
		{Target: "region1.example.com.", Port: 7844},
		{Target: "region2.example.com.", Port: 7844},
	})
	defer restoreSRV()

	restoreIP := stubEdgeLookupIP(map[string][]net.IP{
		"region1.example.com.": {
			net.ParseIP("2001:db8::10"),
			net.ParseIP("198.51.100.10"),
		},
		"region2.example.com.": {
			net.ParseIP("2001:db8::20"),
			net.ParseIP("198.51.100.20"),
		},
	})
	defer restoreIP()

	provider := NewCloudflareEdgeAddressProvider("", EdgeIPv4Only, testEdgeLogger())
	addr, err := provider.ResolveQUICAddress()
	if err != nil {
		t.Fatalf("resolve quic address: %v", err)
	}
	if !strings.Contains(addr, "198.51.100.") {
		t.Fatalf("expected ipv4 address, got %s", addr)
	}
}

func TestCloudflareEdgeAddressProviderResolveQUICAddressRejectsNoUsableAddress(t *testing.T) {
	t.Parallel()

	restoreSRV := stubEdgeLookupSRV([]*net.SRV{
		{Target: "region1.example.com.", Port: 7844},
		{Target: "region2.example.com.", Port: 7844},
	})
	defer restoreSRV()

	restoreIP := stubEdgeLookupIP(map[string][]net.IP{
		"region1.example.com.": {
			net.ParseIP("2001:db8::10"),
		},
		"region2.example.com.": {
			net.ParseIP("2001:db8::20"),
		},
	})
	defer restoreIP()

	provider := NewCloudflareEdgeAddressProvider("", EdgeIPv4Only, testEdgeLogger())
	_, err := provider.ResolveQUICAddress()
	if err == nil {
		t.Fatal("expected error for missing ipv4 edge address")
	}
	if !strings.Contains(err.Error(), "no usable edge address") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stubEdgeLookupSRV(records []*net.SRV) func() {
	original := netLookupSRV
	netLookupSRV = func(service, proto, name string) (string, []*net.SRV, error) {
		return "", records, nil
	}
	return func() {
		netLookupSRV = original
	}
}

func stubEdgeLookupIP(records map[string][]net.IP) func() {
	original := netLookupIP
	netLookupIP = func(host string) ([]net.IP, error) {
		return records[host], nil
	}
	return func() {
		netLookupIP = original
	}
}

func testEdgeLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
