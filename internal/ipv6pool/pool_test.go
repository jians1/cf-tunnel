package ipv6pool

import (
	"net"
	"testing"
)

func TestNewPoolRequiresSource(t *testing.T) {
	t.Parallel()

	_, err := NewPool("", "", "random")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewPoolRejectsIPv4CIDR(t *testing.T) {
	t.Parallel()

	_, err := NewPool("192.0.2.0/24", "", "random")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRandomIPv6FromCIDRStaysInRange(t *testing.T) {
	t.Parallel()

	_, network, err := net.ParseCIDR("2001:db8::/120")
	if err != nil {
		t.Fatalf("parse cidr: %v", err)
	}

	for range 20 {
		ip, err := randomIPv6FromCIDR(network)
		if err != nil {
			t.Fatalf("random ip: %v", err)
		}
		if !network.Contains(ip) {
			t.Fatalf("ip %s not in network %s", ip, network.String())
		}
	}
}

func TestPoolNextIPFromCIDRReturnsIPv6(t *testing.T) {
	t.Parallel()

	pool, err := NewPool("2001:db8::/120", "", "random")
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}

	ip, err := pool.NextIP()
	if err != nil {
		t.Fatalf("next ip: %v", err)
	}
	if ip.To16() == nil || ip.To4() != nil {
		t.Fatalf("expected IPv6, got %v", ip)
	}
}
