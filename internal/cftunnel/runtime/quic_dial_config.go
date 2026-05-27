package runtime

import (
	"crypto/tls"
	"fmt"
	"time"
)

type QUICDialConfig struct {
	Address         string
	AddressProvider EdgeAddressProvider
	ResolvedAddress string
	TLSConfig       *tls.Config
	Timeout         time.Duration
}

func NewQUICDialConfig(prepared *PreparedRuntime, address string, timeout time.Duration) (*QUICDialConfig, error) {
	return NewQUICDialConfigWithProvider(prepared, address, nil, timeout)
}

func NewQUICDialConfigWithProvider(prepared *PreparedRuntime, address string, provider EdgeAddressProvider, timeout time.Duration) (*QUICDialConfig, error) {
	if prepared == nil {
		return nil, fmt.Errorf("nil prepared runtime")
	}
	if address == "" && provider == nil {
		return nil, fmt.Errorf("empty quic dial address")
	}

	tlsConfig, ok := prepared.EdgeTLSByProto["quic"]
	if !ok || tlsConfig == nil {
		return nil, fmt.Errorf("missing quic edge tls config")
	}
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &QUICDialConfig{
		Address:         address,
		AddressProvider: provider,
		TLSConfig:       tlsConfig.Clone(),
		Timeout:         timeout,
	}, nil
}

func (c *QUICDialConfig) resolveAddress() (string, error) {
	if c == nil {
		return "", fmt.Errorf("nil quic dial config")
	}
	if c.ResolvedAddress != "" {
		return c.ResolvedAddress, nil
	}
	if c.Address != "" {
		c.ResolvedAddress = c.Address
		return c.ResolvedAddress, nil
	}
	if c.AddressProvider == nil {
		return "", fmt.Errorf("empty quic dial address")
	}
	address, err := c.AddressProvider.ResolveQUICAddress()
	if err != nil {
		return "", err
	}
	if address == "" {
		return "", fmt.Errorf("resolved empty quic dial address")
	}
	c.ResolvedAddress = address
	return address, nil
}
