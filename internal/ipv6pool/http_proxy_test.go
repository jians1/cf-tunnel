package ipv6pool

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type netDialerAdapter struct {
	d net.Dialer
}

func (a netDialerAdapter) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return a.d.DialContext(ctx, network, address)
}

type testHTTPServer struct {
	ln     net.Listener
	server *http.Server
	URL    string
}

func (s *testHTTPServer) Close() {
	_ = s.server.Close()
	_ = s.ln.Close()
}

func TestHTTPProxyForward(t *testing.T) {
	t.Parallel()

	upstream := startHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "ok")
		_, _ = w.Write([]byte("proxied"))
	}))
	defer upstream.Close()

	proxy := NewHTTPProxy("127.0.0.1:0", netDialerAdapter{}, slog.Default())
	server := startHTTPServer(t, proxy)
	defer server.Close()

	proxyURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}

	proxyClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	resp, err := proxyClient.Get(upstream.URL)
	if err != nil {
		t.Fatalf("proxy get: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := string(body); got != "proxied" {
		t.Fatalf("unexpected body: %q", got)
	}
	if got := resp.Header.Get("X-Upstream"); got != "ok" {
		t.Fatalf("unexpected header: %q", got)
	}
}

func TestHTTPProxyConnect(t *testing.T) {
	t.Parallel()

	upstream, targetAddr := startTCPServer(t, func(conn net.Conn) {
		defer conn.Close()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		if string(buf) != "ping" {
			return
		}
		_, _ = conn.Write([]byte("pong"))
	})
	defer upstream.Close()

	proxy := NewHTTPProxy("127.0.0.1:0", netDialerAdapter{}, slog.Default())
	server := startHTTPServer(t, proxy)
	defer server.Close()

	proxyConn, err := net.DialTimeout("tcp", trimHTTPPrefix(server.URL), 5*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer proxyConn.Close()

	_, err = proxyConn.Write([]byte("CONNECT " + targetAddr + " HTTP/1.1\r\nHost: " + targetAddr + "\r\n\r\n"))
	if err != nil {
		t.Fatalf("write connect request: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(proxyConn), nil)
	if err != nil {
		t.Fatalf("read connect response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	if _, err := proxyConn.Write([]byte("ping")); err != nil {
		t.Fatalf("write tunneled payload: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(proxyConn, buf); err != nil {
		t.Fatalf("read tunneled payload: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("unexpected tunneled payload: %q", string(buf))
	}
}

func trimHTTPPrefix(raw string) string {
	return strings.TrimPrefix(strings.TrimPrefix(raw, "http://"), "https://")
}

func startTCPServer(t *testing.T, handle func(net.Conn)) (net.Listener, string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(conn)
		}
	}()

	return ln, ln.Addr().String()
}

func startHTTPServer(t *testing.T, handler http.Handler) *testHTTPServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen http: %v", err)
	}

	srv := &http.Server{Handler: handler}
	go func() {
		_ = srv.Serve(ln)
	}()

	return &testHTTPServer{
		ln:     ln,
		server: srv,
		URL:    "http://" + ln.Addr().String(),
	}
}
