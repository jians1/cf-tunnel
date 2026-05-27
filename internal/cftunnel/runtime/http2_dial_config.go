package runtime

import (
	"crypto/tls"
	"fmt"
	"time"
)

type HTTP2DialConfig struct {
	Address         string
	AddressProvider EdgeAddressProvider
	ResolvedAddress string
	TLSConfig       *tls.Config
	Timeout         time.Duration
}

func NewHTTP2DialConfig(prepared *PreparedRuntime, address string, timeout time.Duration) (*HTTP2DialConfig, error) {
	return NewHTTP2DialConfigWithProvider(prepared, address, nil, timeout)
}

func NewHTTP2DialConfigWithProvider(prepared *PreparedRuntime, address string, provider EdgeAddressProvider, timeout time.Duration) (*HTTP2DialConfig, error) {
	if prepared == nil {
		return nil, fmt.Errorf("nil prepared runtime")
	}
	if address == "" && provider == nil {
		return nil, fmt.Errorf("empty http2 dial address")
	}

	tlsConfig, ok := prepared.EdgeTLSByProto["http2"]
	if !ok || tlsConfig == nil {
		return nil, fmt.Errorf("missing http2 edge tls config")
	}
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &HTTP2DialConfig{
		Address:         address,
		AddressProvider: provider,
		TLSConfig:       tlsConfig.Clone(),
		Timeout:         timeout,
	}, nil
}

func (c *HTTP2DialConfig) TransportFactory() (HTTP2TransportFactory, error) {
	if c == nil {
		return nil, fmt.Errorf("nil http2 dial config")
	}
	address, err := c.resolveAddress()
	if err != nil {
		return nil, err
	}
	return DialHTTP2TransportFactory{
		Address:   address,
		TLSConfig: c.TLSConfig,
		Timeout:   c.Timeout,
	}, nil
}

func (c *HTTP2DialConfig) resolveAddress() (string, error) {
	if c == nil {
		return "", fmt.Errorf("nil http2 dial config")
	}
	if c.ResolvedAddress != "" {
		return c.ResolvedAddress, nil
	}
	if c.Address != "" {
		c.ResolvedAddress = c.Address
		return c.ResolvedAddress, nil
	}
	if c.AddressProvider == nil {
		return "", fmt.Errorf("empty http2 dial address")
	}
	address, err := c.AddressProvider.ResolveHTTP2Address()
	if err != nil {
		return "", err
	}
	if address == "" {
		return "", fmt.Errorf("resolved empty http2 dial address")
	}
	c.ResolvedAddress = address
	return address, nil
}
