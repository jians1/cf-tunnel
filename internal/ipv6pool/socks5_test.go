package ipv6pool

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestSOCKS5ProxyConnect(t *testing.T) {
	t.Parallel()

	upstream, targetAddr := startTCPServer(t, func(conn net.Conn) {
		defer conn.Close()
		buf := make([]byte, 5)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		if string(buf) != "hello" {
			return
		}
		_, _ = conn.Write([]byte("world"))
	})
	defer upstream.Close()

	listenAddr := allocateTCPAddress(t)
	proxy := NewSOCKS5Proxy(listenAddr, netDialerAdapter{}, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- proxy.Run(ctx)
	}()

	waitForListener(t, listenAddr)

	conn, err := net.DialTimeout("tcp", listenAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("dial socks5: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("write greeting: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("read greeting reply: %v", err)
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		t.Fatalf("unexpected greeting reply: %v", reply)
	}

	host, port, err := net.SplitHostPort(targetAddr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("atoi port: %v", err)
	}

	req := []byte{0x05, 0x01, 0x00, 0x01}
	req = append(req, net.ParseIP(host).To4()...)
	req = append(req, byte(portNum>>8), byte(portNum))
	if _, err := conn.Write(req); err != nil {
		t.Fatalf("write connect request: %v", err)
	}

	resp := make([]byte, 10)
	if _, err := io.ReadFull(conn, resp); err != nil {
		t.Fatalf("read connect reply: %v", err)
	}
	if resp[1] != 0x00 {
		t.Fatalf("unexpected connect reply: %v", resp)
	}

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write tunneled payload: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read tunneled payload: %v", err)
	}
	if string(buf) != "world" {
		t.Fatalf("unexpected tunneled payload: %q", string(buf))
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("proxy run error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for socks5 proxy shutdown")
	}
}

func allocateTCPAddress(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate tcp address: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func waitForListener(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("listener did not become ready: %s", addr)
}
