package runtime

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"

	cfdedgediscovery "github.com/cloudflare/cloudflared/edgediscovery"
	"github.com/cloudflare/cloudflared/edgediscovery/allregions"
	"github.com/rs/zerolog"
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
	Region    string
	IPVersion allregions.ConfigIPVersion
	logger    *zerolog.Logger
}

func NewCloudflareEdgeAddressProvider(region string, ipVersion allregions.ConfigIPVersion, logger *slog.Logger) *CloudflareEdgeAddressProvider {
	zlog := newZeroLoggerFromSlog(logger)
	return &CloudflareEdgeAddressProvider{
		Region:    region,
		IPVersion: ipVersion,
		logger:    &zlog,
	}
}

func (p *CloudflareEdgeAddressProvider) ResolveHTTP2Address() (string, error) {
	addr, err := p.resolveEdgeAddr()
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(addr.TCP.IP.String(), strconv.Itoa(addr.TCP.Port)), nil
}

func (p *CloudflareEdgeAddressProvider) ResolveQUICAddress() (string, error) {
	addr, err := p.resolveEdgeAddr()
	if err != nil {
		return "", err
	}
	if addr.UDP == nil {
		return "", fmt.Errorf("edge discovery returned empty udp address")
	}
	return net.JoinHostPort(addr.UDP.IP.String(), strconv.Itoa(addr.UDP.Port)), nil
}

func (p *CloudflareEdgeAddressProvider) resolveEdgeAddr() (*allregions.EdgeAddr, error) {
	if p == nil {
		return nil, fmt.Errorf("nil cloudflare edge address provider")
	}
	edge, err := cfdedgediscovery.ResolveEdge(p.logger, p.Region, p.IPVersion)
	if err != nil {
		return nil, err
	}
	addr, err := edge.GetAddrForRPC()
	if err != nil {
		return nil, err
	}
	if addr == nil {
		return nil, fmt.Errorf("edge discovery returned empty address")
	}
	return addr, nil
}
