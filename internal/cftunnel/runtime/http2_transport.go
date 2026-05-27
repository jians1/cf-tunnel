package runtime

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"
)

type HTTP2Transport struct {
	edgeConn   net.Conn
	serverConn net.Conn
	closeFn    func() error
}

func (t *HTTP2Transport) EdgeConn() net.Conn {
	if t == nil {
		return nil
	}
	return t.edgeConn
}

func (t *HTTP2Transport) ServerConn() net.Conn {
	if t == nil {
		return nil
	}
	return t.serverConn
}

func (t *HTTP2Transport) Close() error {
	if t == nil {
		return nil
	}

	var firstErr error
	if t.edgeConn != nil {
		if err := t.edgeConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if t.serverConn != nil {
		if err := t.serverConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if t.closeFn != nil {
		if err := t.closeFn(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type HTTP2TransportFactory interface {
	NewTransport(ctx context.Context) (*HTTP2Transport, error)
}

type PipeHTTP2TransportFactory struct{}

func (PipeHTTP2TransportFactory) NewTransport(context.Context) (*HTTP2Transport, error) {
	edgeConn, serverConn := net.Pipe()
	return &HTTP2Transport{
		edgeConn:   edgeConn,
		serverConn: serverConn,
		closeFn:    func() error { return nil },
	}, nil
}

type LoopbackHTTP2TransportFactory struct{}

func (LoopbackHTTP2TransportFactory) NewTransport(context.Context) (*HTTP2Transport, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen loopback transport: %w", err)
	}

	type acceptResult struct {
		conn net.Conn
		err  error
	}
	acceptCh := make(chan acceptResult, 1)
	go func() {
		conn, err := listener.Accept()
		acceptCh <- acceptResult{conn: conn, err: err}
	}()

	edgeConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("dial loopback transport: %w", err)
	}

	result := <-acceptCh
	if result.err != nil {
		_ = edgeConn.Close()
		_ = listener.Close()
		return nil, fmt.Errorf("accept loopback transport: %w", result.err)
	}

	var once sync.Once
	closeFn := func() error {
		var closeErr error
		once.Do(func() {
			if err := listener.Close(); err != nil {
				closeErr = err
			}
		})
		return closeErr
	}

	return &HTTP2Transport{
		edgeConn:   edgeConn,
		serverConn: result.conn,
		closeFn:    closeFn,
	}, nil
}

type DialHTTP2TransportFactory struct {
	Address   string
	TLSConfig *tls.Config
	Timeout   time.Duration
}

func (f DialHTTP2TransportFactory) NewTransport(ctx context.Context) (*HTTP2Transport, error) {
	if f.Address == "" {
		return nil, fmt.Errorf("empty dial transport address")
	}
	timeout := f.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	dialer := &net.Dialer{Timeout: timeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", f.Address)
	if err != nil {
		return nil, fmt.Errorf("dial http2 transport: %w", err)
	}

	if f.TLSConfig != nil {
		tlsConn := tls.Client(rawConn, f.TLSConfig.Clone())
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = rawConn.Close()
			return nil, fmt.Errorf("handshake http2 transport: %w", err)
		}
		return &HTTP2Transport{
			edgeConn:   tlsConn,
			serverConn: nil,
			closeFn:    func() error { return nil },
		}, nil
	}

	return &HTTP2Transport{
		edgeConn:   rawConn,
		serverConn: nil,
		closeFn:    func() error { return nil },
	}, nil
}
