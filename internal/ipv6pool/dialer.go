package ipv6pool

import (
	"context"
	"fmt"
	"net"
	"time"
)

type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type Dialer struct {
	pool *Pool
}

func NewDialer(pool *Pool) *Dialer {
	return &Dialer{pool: pool}
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	ip, err := d.pool.NextIP()
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		LocalAddr: &net.TCPAddr{IP: ip},
	}

	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, fmt.Errorf("dial %s via %s: %w", address, ip.String(), err)
	}
	return conn, nil
}
