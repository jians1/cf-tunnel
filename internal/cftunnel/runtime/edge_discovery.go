package runtime

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"
)

const (
	edgeSRVProto          = "tcp"
	edgeSRVName           = "argotunnel.com"
	edgeSRVService        = "v2-origintunneld"
	edgeDOTServerName     = "cloudflare-dns.com"
	edgeDOTServerAddr     = "1.1.1.1:853"
	edgeDOTTimeout        = 15 * time.Second
	edgeResolveMinRegions = 2
)

var (
	netLookupSRV      = net.LookupSRV
	netLookupIP       = net.LookupIP
	fallbackLookupSRV = lookupSRVWithDOT
)

type EdgeIPVersion int8

const (
	EdgeIPAuto   EdgeIPVersion = 2
	EdgeIPv4Only EdgeIPVersion = 4
	EdgeIPv6Only EdgeIPVersion = 6
)

type EdgeAddressProvider interface {
	ResolveHTTP2Address() (string, error)
	ResolveQUICAddress() (string, error)
}

type StaticEdgeAddressProvider struct {
	Address     string
	QUICAddress string
}

func (p StaticEdgeAddressProvider) ResolveHTTP2Address() (string, error) {
	if p.Address == "" {
		return "", fmt.Errorf("empty static edge address")
	}
	return p.Address, nil
}

func (p StaticEdgeAddressProvider) ResolveQUICAddress() (string, error) {
	if p.QUICAddress != "" {
		return p.QUICAddress, nil
	}
	if p.Address == "" {
		return "", fmt.Errorf("empty static quic edge address")
	}
	return p.Address, nil
}

type CloudflareEdgeAddressProvider struct {
	ConnIndex uint8
	Region    string
	IPVersion EdgeIPVersion
	logger    *slog.Logger
}

func NewCloudflareEdgeAddressProvider(region string, ipVersion EdgeIPVersion, logger *slog.Logger) *CloudflareEdgeAddressProvider {
	return &CloudflareEdgeAddressProvider{
		Region:    region,
		IPVersion: ipVersion,
		logger:    logger,
	}
}

func (p *CloudflareEdgeAddressProvider) ForConnIndex(connIndex uint8) *CloudflareEdgeAddressProvider {
	if p == nil {
		return nil
	}
	clone := *p
	clone.ConnIndex = connIndex
	return &clone
}

func (p *CloudflareEdgeAddressProvider) ResolveHTTP2Address() (string, error) {
	addr, err := p.resolveEdgeAddr()
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(addr.IP.String(), strconv.Itoa(addr.Port)), nil
}

func (p *CloudflareEdgeAddressProvider) ResolveQUICAddress() (string, error) {
	addr, err := p.resolveEdgeAddr()
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(addr.IP.String(), strconv.Itoa(addr.Port)), nil
}

func (p *CloudflareEdgeAddressProvider) resolveEdgeAddr() (*net.TCPAddr, error) {
	addrs, err := p.resolveAllAddrs()
	if err != nil {
		return nil, err
	}
	return addrs[int(p.ConnIndex)%len(addrs)], nil
}

// resolveAllAddrs returns every usable edge address for the configured region
// and IP version. The shared edge address pool uses it to hand out distinct
// addresses per connection index instead of the modulo selection above.
func (p *CloudflareEdgeAddressProvider) resolveAllAddrs() ([]*net.TCPAddr, error) {
	if p == nil {
		return nil, fmt.Errorf("nil cloudflare edge address provider")
	}

	service := edgeRegionalServiceName(p.Region)
	_, records, err := netLookupSRV(service, edgeSRVProto, edgeSRVName)
	if err != nil {
		_, records, err = fallbackLookupSRV(service, edgeSRVProto, edgeSRVName)
		if err != nil {
			return nil, fmt.Errorf("lookup edge srv records: %w", err)
		}
	}
	if len(records) < edgeResolveMinRegions {
		return nil, fmt.Errorf("expected at least %d edge regions, got %d", edgeResolveMinRegions, len(records))
	}

	addrs := make([]*net.TCPAddr, 0, len(records))
	for _, record := range records {
		recordAddrs, err := p.resolveRecordAddrs(record)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, recordAddrs...)
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("no usable edge address for ip version %d", p.IPVersion)
	}

	return addrs, nil
}

func (p *CloudflareEdgeAddressProvider) resolveRecordAddrs(record *net.SRV) ([]*net.TCPAddr, error) {
	ips, err := netLookupIP(record.Target)
	if err != nil {
		return nil, fmt.Errorf("resolve edge ip for %s: %w", record.Target, err)
	}
	addrs := make([]*net.TCPAddr, 0, len(ips))
	for _, ip := range ips {
		if edgeIPMatchesVersion(ip, p.IPVersion) {
			addrs = append(addrs, &net.TCPAddr{IP: ip, Port: int(record.Port)})
		}
	}
	return addrs, nil
}

func edgeRegionalServiceName(region string) string {
	if region == "" {
		return edgeSRVService
	}
	return region + "-" + edgeSRVService
}

func edgeIPMatchesVersion(ip net.IP, version EdgeIPVersion) bool {
	isIPv4 := ip.To4() != nil
	switch version {
	case EdgeIPv4Only:
		return isIPv4
	case EdgeIPv6Only:
		return !isIPv4
	case EdgeIPAuto:
		return true
	default:
		return true
	}
}

func lookupSRVWithDOT(service, proto, name string) (string, []*net.SRV, error) {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, "tcp", edgeDOTServerAddr)
			if err != nil {
				return nil, err
			}
			return tls.Client(conn, &tls.Config{ServerName: edgeDOTServerName}), nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), edgeDOTTimeout)
	defer cancel()
	return resolver.LookupSRV(ctx, service, proto, name)
}
