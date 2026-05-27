package ipv6pool

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	socksVersion5       = 0x05
	socksAuthNone       = 0x00
	socksCmdConnect     = 0x01
	socksAddrTypeIPv4   = 0x01
	socksAddrTypeDomain = 0x03
	socksAddrTypeIPv6   = 0x04
)

type SOCKS5Proxy struct {
	listen string
	dialer ContextDialer
	logger *slog.Logger
}

func NewSOCKS5Proxy(listen string, dialer ContextDialer, logger *slog.Logger) *SOCKS5Proxy {
	return &SOCKS5Proxy{
		listen: listen,
		dialer: dialer,
		logger: logger.With("proxy", "socks5"),
	}
}

func (p *SOCKS5Proxy) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", p.listen)
	if err != nil {
		return err
	}
	defer ln.Close()

	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	go func() {
		p.logger.Info("socks5 proxy listening", "listen", p.listen)
		for {
			conn, err := ln.Accept()
			if err != nil {
				errCh <- err
				return
			}
			go p.handleConn(ctx, conn)
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
}

func (p *SOCKS5Proxy) handleConn(ctx context.Context, client net.Conn) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(30 * time.Second))

	if err := p.handshake(client); err != nil {
		p.logger.Debug("socks5 handshake failed", "error", err)
		return
	}

	target, err := readSOCKS5Request(client)
	if err != nil {
		p.logger.Debug("socks5 request failed", "error", err)
		return
	}

	targetConn, err := p.dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		_, _ = client.Write([]byte{socksVersion5, 0x05, 0x00, socksAddrTypeIPv4, 0, 0, 0, 0, 0, 0})
		p.logger.Error("socks5 connect failed", "target", target, "error", err)
		return
	}
	defer targetConn.Close()

	_ = client.SetDeadline(time.Time{})
	if _, err := client.Write([]byte{socksVersion5, 0x00, 0x00, socksAddrTypeIPv4, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	group, _ := errgroup.WithContext(ctx)
	group.Go(func() error {
		defer client.Close()
		defer targetConn.Close()
		_, err := io.Copy(targetConn, client)
		return err
	})
	group.Go(func() error {
		defer client.Close()
		defer targetConn.Close()
		_, err := io.Copy(client, targetConn)
		return err
	})
	_ = group.Wait()
}

func (p *SOCKS5Proxy) handshake(conn net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[0] != socksVersion5 {
		return fmt.Errorf("unsupported socks version: %d", header[0])
	}

	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return err
	}

	for _, method := range methods {
		if method == socksAuthNone {
			_, err := conn.Write([]byte{socksVersion5, socksAuthNone})
			return err
		}
	}

	_, err := conn.Write([]byte{socksVersion5, 0xff})
	return err
}

func readSOCKS5Request(conn net.Conn) (string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}
	if header[0] != socksVersion5 {
		return "", fmt.Errorf("unsupported socks version: %d", header[0])
	}
	if header[1] != socksCmdConnect {
		return "", fmt.Errorf("unsupported socks command: %d", header[1])
	}

	host, err := readSOCKS5Address(conn, header[3])
	if err != nil {
		return "", err
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return "", err
	}
	port := strconv.Itoa(int(binary.BigEndian.Uint16(portBytes)))
	return net.JoinHostPort(host, port), nil
}

func readSOCKS5Address(conn net.Conn, atyp byte) (string, error) {
	switch atyp {
	case socksAddrTypeIPv4:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case socksAddrTypeIPv6:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case socksAddrTypeDomain:
		size := make([]byte, 1)
		if _, err := io.ReadFull(conn, size); err != nil {
			return "", err
		}
		buf := make([]byte, int(size[0]))
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	default:
		return "", fmt.Errorf("unsupported socks address type: %d", atyp)
	}
}
