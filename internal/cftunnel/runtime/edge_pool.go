package runtime

import (
	"fmt"
	"net"
	"strconv"
	"sync"
)

// edgeAddressPool hands out edge addresses so that each HA connection index is
// bound to a distinct edge server, mirroring the upstream cloudflared
// edgediscovery.Edge / allregions bookkeeping. Selecting distinct addresses is
// what prevents the edge from rejecting a second connection on the same server
// with a duplicate-connection (EDUPCONN) error.
//
// The pool resolves the full address set once (lazily, under the lock) and then
// tracks which connection index currently holds which address. addrFor is
// sticky: a connection index keeps its address until rotate releases it.
type edgeAddressPool struct {
	mu       sync.Mutex
	resolve  func() ([]*net.TCPAddr, error)
	addrs    []*net.TCPAddr
	usedBy   map[uint8]*net.TCPAddr
	resolved bool
}

func newEdgeAddressPool(resolve func() ([]*net.TCPAddr, error)) *edgeAddressPool {
	return &edgeAddressPool{
		resolve: resolve,
		usedBy:  make(map[uint8]*net.TCPAddr),
	}
}

// newCloudflareEdgeAddressPool builds a pool backed by the given Cloudflare edge
// address provider. It returns nil when the provider cannot enumerate its full
// address set, letting callers fall back to the per-connection provider path.
func newCloudflareEdgeAddressPool(provider *CloudflareEdgeAddressProvider) *edgeAddressPool {
	if provider == nil {
		return nil
	}
	return newEdgeAddressPool(provider.resolveAllAddrs)
}

func (p *edgeAddressPool) ensureResolved() error {
	if p.resolved {
		if len(p.addrs) == 0 {
			return fmt.Errorf("no usable edge addresses")
		}
		return nil
	}
	if p.resolve == nil {
		return fmt.Errorf("nil edge address resolver")
	}
	addrs, err := p.resolve()
	if err != nil {
		return err
	}
	p.addrs = addrs
	p.resolved = true
	if len(p.addrs) == 0 {
		return fmt.Errorf("no usable edge addresses")
	}
	return nil
}

// addrFor returns the address bound to connIndex, allocating an unused one on
// first use. When every distinct address is already in use it falls back to a
// deterministic modulo selection so more connections than addresses still work.
func (p *edgeAddressPool) addrFor(connIndex uint8) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureResolved(); err != nil {
		return "", err
	}
	if addr, ok := p.usedBy[connIndex]; ok {
		return addrString(addr), nil
	}
	addr := p.unusedAddrLocked(nil)
	if addr == nil {
		// More connections than distinct addresses: reuse deterministically.
		addr = p.addrs[int(connIndex)%len(p.addrs)]
	}
	p.usedBy[connIndex] = addr
	return addrString(addr), nil
}

// rotate releases the address currently held by connIndex and binds a different
// one, mirroring cloudflared's GetDifferentAddr. The next addrFor call reflects
// the new binding.
func (p *edgeAddressPool) rotate(connIndex uint8) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.resolved || len(p.addrs) == 0 {
		return
	}
	old := p.usedBy[connIndex]
	delete(p.usedBy, connIndex)

	// Exclude the just-released address so rotate prefers a genuinely
	// different edge server, mirroring cloudflared's GetUnusedAddr(oldAddr).
	addr := p.unusedAddrLocked(old)
	if addr == nil {
		// Every other address is already in use, so the freed address is the
		// best remaining choice; reusing it avoids doubling up on a server that
		// another connection holds (which would itself trigger EDUPCONN). This
		// mirrors cloudflared, where oldAddr becomes available again on
		// exhaustion.
		addr = old
	}
	if addr != nil {
		p.usedBy[connIndex] = addr
	}
}

// unusedAddrLocked returns an address not currently bound to any connection
// index and not equal to exclude, or nil when none qualify. Pass nil to place
// no exclusion.
func (p *edgeAddressPool) unusedAddrLocked(exclude *net.TCPAddr) *net.TCPAddr {
	inUse := make(map[string]struct{}, len(p.usedBy))
	for _, addr := range p.usedBy {
		inUse[addrString(addr)] = struct{}{}
	}
	if exclude != nil {
		inUse[addrString(exclude)] = struct{}{}
	}
	for _, addr := range p.addrs {
		if _, taken := inUse[addrString(addr)]; !taken {
			return addr
		}
	}
	return nil
}

func (p *edgeAddressPool) providerFor(connIndex uint8) EdgeAddressProvider {
	return pooledEdgeAddressProvider{pool: p, connIndex: connIndex}
}

func addrString(addr *net.TCPAddr) string {
	return net.JoinHostPort(addr.IP.String(), strconv.Itoa(addr.Port))
}

// pooledEdgeAddressProvider resolves the address the pool currently assigns to
// its connection index. It satisfies EdgeAddressProvider so it drops straight
// into the existing dial-config plumbing.
type pooledEdgeAddressProvider struct {
	pool      *edgeAddressPool
	connIndex uint8
}

func (p pooledEdgeAddressProvider) ResolveHTTP2Address() (string, error) {
	return p.pool.addrFor(p.connIndex)
}

func (p pooledEdgeAddressProvider) ResolveQUICAddress() (string, error) {
	return p.pool.addrFor(p.connIndex)
}
