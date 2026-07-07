package runtime

import (
	"errors"
	"net"
	"testing"
)

func testEdgeAddrs(ips ...string) []*net.TCPAddr {
	addrs := make([]*net.TCPAddr, 0, len(ips))
	for _, ip := range ips {
		addrs = append(addrs, &net.TCPAddr{IP: net.ParseIP(ip), Port: 7844})
	}
	return addrs
}

func TestEdgeAddressPoolAssignsDistinctAddresses(t *testing.T) {
	t.Parallel()

	pool := newEdgeAddressPool(func() ([]*net.TCPAddr, error) {
		return testEdgeAddrs("198.51.100.10", "198.51.100.20", "198.51.100.30", "198.51.100.40"), nil
	})

	seen := make(map[string]uint8)
	for connIndex := uint8(0); connIndex < 4; connIndex++ {
		addr, err := pool.addrFor(connIndex)
		if err != nil {
			t.Fatalf("addrFor(%d): %v", connIndex, err)
		}
		if prev, ok := seen[addr]; ok {
			t.Fatalf("address %s handed to both conn %d and conn %d", addr, prev, connIndex)
		}
		seen[addr] = connIndex
	}
}

func TestEdgeAddressPoolIsStickyPerConnIndex(t *testing.T) {
	t.Parallel()

	pool := newEdgeAddressPool(func() ([]*net.TCPAddr, error) {
		return testEdgeAddrs("198.51.100.10", "198.51.100.20"), nil
	})

	first, err := pool.addrFor(0)
	if err != nil {
		t.Fatalf("addrFor: %v", err)
	}
	again, err := pool.addrFor(0)
	if err != nil {
		t.Fatalf("addrFor: %v", err)
	}
	if first != again {
		t.Fatalf("expected sticky address, got %s then %s", first, again)
	}
}

func TestEdgeAddressPoolRotatePicksDifferentAddress(t *testing.T) {
	t.Parallel()

	pool := newEdgeAddressPool(func() ([]*net.TCPAddr, error) {
		return testEdgeAddrs("198.51.100.10", "198.51.100.20"), nil
	})

	before, err := pool.addrFor(0)
	if err != nil {
		t.Fatalf("addrFor: %v", err)
	}
	pool.rotate(0)
	after, err := pool.addrFor(0)
	if err != nil {
		t.Fatalf("addrFor: %v", err)
	}
	if before == after {
		t.Fatalf("rotate did not change address: still %s", before)
	}
}

func TestEdgeAddressPoolRotateFreesAddressForReuse(t *testing.T) {
	t.Parallel()

	pool := newEdgeAddressPool(func() ([]*net.TCPAddr, error) {
		return testEdgeAddrs("198.51.100.10", "198.51.100.20"), nil
	})

	// conn 0 and conn 1 take the two distinct addresses.
	addr0, _ := pool.addrFor(0)
	addr1, _ := pool.addrFor(1)
	if addr0 == addr1 {
		t.Fatalf("expected distinct addresses, both got %s", addr0)
	}

	// Rotating conn 0 releases addr0; with only two addresses and conn 1
	// holding addr1, conn 0 must land back on addr0 (the only free one).
	pool.rotate(0)
	rotated, _ := pool.addrFor(0)
	if rotated != addr0 {
		t.Fatalf("expected conn 0 to reuse freed %s, got %s", addr0, rotated)
	}
}

func TestEdgeAddressPoolFallsBackWhenMoreConnsThanAddresses(t *testing.T) {
	t.Parallel()

	pool := newEdgeAddressPool(func() ([]*net.TCPAddr, error) {
		return testEdgeAddrs("198.51.100.10", "198.51.100.20"), nil
	})

	// Three connections, two addresses: all must still get a usable address.
	for connIndex := uint8(0); connIndex < 3; connIndex++ {
		addr, err := pool.addrFor(connIndex)
		if err != nil {
			t.Fatalf("addrFor(%d): %v", connIndex, err)
		}
		if addr == "" {
			t.Fatalf("addrFor(%d) returned empty address", connIndex)
		}
	}
}

func TestEdgeAddressPoolResolvesOnce(t *testing.T) {
	t.Parallel()

	var calls int
	pool := newEdgeAddressPool(func() ([]*net.TCPAddr, error) {
		calls++
		return testEdgeAddrs("198.51.100.10", "198.51.100.20"), nil
	})

	for connIndex := uint8(0); connIndex < 2; connIndex++ {
		if _, err := pool.addrFor(connIndex); err != nil {
			t.Fatalf("addrFor(%d): %v", connIndex, err)
		}
	}
	pool.rotate(0)
	if _, err := pool.addrFor(0); err != nil {
		t.Fatalf("addrFor after rotate: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected resolver to run once, ran %d times", calls)
	}
}

func TestEdgeAddressPoolPropagatesResolveError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("lookup failed")
	pool := newEdgeAddressPool(func() ([]*net.TCPAddr, error) {
		return nil, wantErr
	})

	_, err := pool.addrFor(0)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected resolve error to propagate, got %v", err)
	}
}

func TestBridgeRunnerBuildsEdgePoolFromCloudflareProvider(t *testing.T) {
	// Not parallel: mutates the package-level edge lookup stubs, which the
	// other (non-parallel) edge-discovery tests also rely on.
	restoreSRV := stubEdgeLookupSRV([]*net.SRV{
		{Target: "region1.example.com.", Port: 7844},
		{Target: "region2.example.com.", Port: 7844},
	})
	defer restoreSRV()
	restoreIP := stubEdgeLookupIP(map[string][]net.IP{
		"region1.example.com.": {net.ParseIP("198.51.100.10")},
		"region2.example.com.": {net.ParseIP("198.51.100.20")},
	})
	defer restoreIP()

	runner := NewBridgeRunner(testSession(t, "quic"), newDiscardSlogLogger())
	runner.SetQUICOptions(QUICRuntimeOptions{
		EdgeAddressProvider: NewCloudflareEdgeAddressProvider("", EdgeIPv4Only, newDiscardSlogLogger()),
	})

	runner.ensureEdgePool()
	if runner.edgePool == nil {
		t.Fatal("expected edge pool to be built from cloudflare provider")
	}

	options := runner.instanceOptions(0)
	addr, err := options.QUIC.EdgeAddressProvider.ResolveQUICAddress()
	if err != nil {
		t.Fatalf("resolve quic address: %v", err)
	}
	if addr != "198.51.100.10:7844" && addr != "198.51.100.20:7844" {
		t.Fatalf("unexpected pooled address: %s", addr)
	}
}

func TestBridgeRunnerLeavesEdgePoolNilForCustomProvider(t *testing.T) {
	t.Parallel()

	runner := NewBridgeRunner(testSession(t, "quic"), newDiscardSlogLogger())
	runner.SetQUICOptions(QUICRuntimeOptions{
		EdgeAddressProvider: indexedEdgeAddressProvider{},
	})

	runner.ensureEdgePool()
	if runner.edgePool != nil {
		t.Fatal("expected custom provider to leave edge pool nil")
	}
}

// TestBridgeRunnerInstanceOptionsUsesPoolForDistinctAddresses is the
// regression guard for the wiring bug where the shared pool was built but
// instanceOptions never consulted it. It uses two edge IPs and queries
// connIndex 0 and 2: the old per-connection modulo path (addrs[connIndex%len])
// collides for those indexes, so this test only passes when instanceOptions
// actually routes through the pool, which assigns distinct addresses.
func TestBridgeRunnerInstanceOptionsUsesPoolForDistinctAddresses(t *testing.T) {
	// Not parallel: mutates the package-level edge lookup stubs.
	restoreSRV := stubEdgeLookupSRV([]*net.SRV{
		{Target: "region1.example.com.", Port: 7844},
		{Target: "region2.example.com.", Port: 7844},
	})
	defer restoreSRV()
	restoreIP := stubEdgeLookupIP(map[string][]net.IP{
		"region1.example.com.": {net.ParseIP("198.51.100.10")},
		"region2.example.com.": {net.ParseIP("198.51.100.20")},
	})
	defer restoreIP()

	runner := NewBridgeRunner(testSession(t, "quic"), newDiscardSlogLogger())
	runner.SetQUICOptions(QUICRuntimeOptions{
		EdgeAddressProvider: NewCloudflareEdgeAddressProvider("", EdgeIPv4Only, newDiscardSlogLogger()),
	})
	runner.ensureEdgePool()
	if runner.edgePool == nil {
		t.Fatal("expected edge pool to be built")
	}

	addr0, err := runner.instanceOptions(0).QUIC.EdgeAddressProvider.ResolveQUICAddress()
	if err != nil {
		t.Fatalf("resolve conn 0: %v", err)
	}
	addr2, err := runner.instanceOptions(2).QUIC.EdgeAddressProvider.ResolveQUICAddress()
	if err != nil {
		t.Fatalf("resolve conn 2: %v", err)
	}
	if addr0 == addr2 {
		t.Fatalf("conn 0 and conn 2 got the same edge address %s; pool not wired into instanceOptions", addr0)
	}
}
