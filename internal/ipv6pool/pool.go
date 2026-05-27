package ipv6pool

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net"
)

type Pool struct {
	addresses []net.IP
	cidr      *net.IPNet
	strategy  string
}

func NewPool(cidr, bindInterface, strategy string) (*Pool, error) {
	pool := &Pool{strategy: strategy}

	if bindInterface != "" {
		addresses, err := loadInterfaceIPv6Addresses(bindInterface)
		if err != nil {
			return nil, err
		}
		pool.addresses = append(pool.addresses, addresses...)
	}

	if cidr != "" {
		ip, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("parse cidr: %w", err)
		}
		if ip.To4() != nil {
			return nil, errors.New("ipv6-pool-cidr must be IPv6")
		}
		network.IP = ip.Mask(network.Mask)
		pool.cidr = network
	}

	if len(pool.addresses) == 0 && pool.cidr == nil {
		return nil, errors.New("no IPv6 pool sources available")
	}

	return pool, nil
}

func (p *Pool) NextIP() (net.IP, error) {
	if len(p.addresses) > 0 {
		return cloneIP(p.addresses[randomIndex(len(p.addresses))]), nil
	}
	if p.cidr != nil {
		return randomIPv6FromCIDR(p.cidr)
	}
	return nil, errors.New("no IPv6 address available")
}

func loadInterfaceIPv6Addresses(name string) ([]net.IP, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, fmt.Errorf("load interface %s: %w", name, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("list interface addresses for %s: %w", name, err)
	}

	var out []net.IP
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP.To16()
		if ip == nil || ip.To4() != nil || !ip.IsGlobalUnicast() {
			continue
		}
		out = append(out, cloneIP(ip))
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("interface %s has no global unicast IPv6 addresses", name)
	}
	return out, nil
}

func randomIPv6FromCIDR(network *net.IPNet) (net.IP, error) {
	ones, bits := network.Mask.Size()
	if bits != 128 {
		return nil, errors.New("cidr is not IPv6")
	}
	hostBits := bits - ones
	if hostBits <= 0 {
		return cloneIP(network.IP.To16()), nil
	}

	max := new(big.Int).Lsh(big.NewInt(1), uint(hostBits))
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return nil, fmt.Errorf("generate random IPv6 host: %w", err)
	}

	base := new(big.Int).SetBytes(network.IP.To16())
	base.Add(base, n)

	ip := base.FillBytes(make([]byte, net.IPv6len))
	return net.IP(ip), nil
}

func randomIndex(size int) int {
	if size <= 1 {
		return 0
	}
	max := big.NewInt(int64(size))
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

func cloneIP(ip net.IP) net.IP {
	out := make(net.IP, len(ip))
	copy(out, ip)
	return out
}
