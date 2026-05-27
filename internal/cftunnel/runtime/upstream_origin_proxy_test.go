package runtime

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	cfdconnection "github.com/cloudflare/cloudflared/connection"
	cfdtracing "github.com/cloudflare/cloudflared/tracing"
)

func TestUpstreamOriginProxyProxyHTTP(t *testing.T) {
	t.Parallel()

	proxy := NewUpstreamOriginProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("proxied"))
	}))

	req, err := http.NewRequest(http.MethodGet, "http://example.test/demo", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	logger := newTestZeroLogger()
	tr := cfdtracing.NewTracedHTTPRequest(req, 0, &logger)

	resp := newMockResponseWriter()
	if err := proxy.ProxyHTTP(resp, tr, false); err != nil {
		t.Fatalf("proxy http: %v", err)
	}
	if resp.status != http.StatusCreated {
		t.Fatalf("unexpected status: %d", resp.status)
	}
	if got := resp.header.Get("X-Upstream"); got != "ok" {
		t.Fatalf("unexpected header: %q", got)
	}
	if got := string(resp.body.String()); got != "proxied" {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestUpstreamOriginProxyWritesHeadersOnceForMultipleBodyWrites(t *testing.T) {
	t.Parallel()

	proxy := NewUpstreamOriginProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "6")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("abc"))
		_, _ = w.Write([]byte("def"))
	}))

	req, err := http.NewRequest(http.MethodGet, "http://example.test/demo", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	logger := newTestZeroLogger()
	tr := cfdtracing.NewTracedHTTPRequest(req, 0, &logger)

	resp := newMockResponseWriter()
	if err := proxy.ProxyHTTP(resp, tr, false); err != nil {
		t.Fatalf("proxy http: %v", err)
	}
	if resp.headerWrites != 1 {
		t.Fatalf("unexpected response header writes: %d", resp.headerWrites)
	}
	if got := string(resp.body.String()); got != "abcdef" {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestUpstreamOriginProxyRestoresWebsocketUpgradeHeaders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		internal   bool
		websocket  bool
		shouldKeep bool
	}{
		{name: "internal header", internal: true, shouldKeep: true},
		{name: "websocket flag", websocket: true, shouldKeep: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proxy := NewUpstreamOriginProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Connection"); !strings.EqualFold(got, "Upgrade") {
					t.Fatalf("unexpected connection header: %q", got)
				}
				if got := r.Header.Get("Upgrade"); !strings.EqualFold(got, "websocket") {
					t.Fatalf("unexpected upgrade header: %q", got)
				}
				w.WriteHeader(http.StatusSwitchingProtocols)
			}))

			req, err := http.NewRequest(http.MethodGet, "http://example.test/vless-ws", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			if tc.internal {
				req.Header.Set(cfdconnection.InternalUpgradeHeader, cfdconnection.WebsocketUpgrade)
			}
			logger := newTestZeroLogger()
			tr := cfdtracing.NewTracedHTTPRequest(req, 0, &logger)

			resp := newMockResponseWriter()
			if err := proxy.ProxyHTTP(resp, tr, tc.websocket); err != nil {
				t.Fatalf("proxy websocket: %v", err)
			}
			if resp.status != http.StatusSwitchingProtocols {
				t.Fatalf("unexpected status: %d", resp.status)
			}
		})
	}
}

func TestUpstreamOriginProxyWritesSwitchingProtocolsBeforeWebsocketHijack(t *testing.T) {
	t.Parallel()

	proxy := NewUpstreamOriginProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("response writer does not implement hijacker")
		}
		_, _, _ = hijacker.Hijack()
	}))

	req, err := http.NewRequest(http.MethodGet, "http://example.test/vless-ws", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	logger := newTestZeroLogger()
	tr := cfdtracing.NewTracedHTTPRequest(req, 0, &logger)

	resp := newMockResponseWriter()
	if err := proxy.ProxyHTTP(resp, tr, true); err != nil {
		t.Fatalf("proxy websocket: %v", err)
	}
	if resp.status != http.StatusSwitchingProtocols {
		t.Fatalf("unexpected status before hijack: %d", resp.status)
	}
}

func TestUpstreamOriginProxyProxyTCPForwardsBytes(t *testing.T) {
	t.Parallel()

	proxy := NewUpstreamOriginProxy(http.NotFoundHandler())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	originDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			originDone <- err
			return
		}
		defer conn.Close()

		buf := make([]byte, len("ping"))
		if _, err := io.ReadFull(conn, buf); err != nil {
			originDone <- err
			return
		}
		if string(buf) != "ping" {
			originDone <- errors.New("unexpected origin payload: " + string(buf))
			return
		}
		_, err = conn.Write([]byte("pong"))
		originDone <- err
	}()

	edge, client := net.Pipe()
	defer client.Close()
	rwa := newMockReadWriteAcker(edge)

	proxyDone := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		proxyDone <- proxy.ProxyTCP(ctx, rwa, &cfdconnection.TCPRequest{Dest: listener.Addr().String()})
	}()

	select {
	case <-rwa.ackC:
	case err := <-proxyDone:
		t.Fatalf("proxy tcp returned before acknowledging connection: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("proxy tcp did not acknowledge connection")
	}

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write client payload: %v", err)
	}
	buf := make([]byte, len("pong"))
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("read client payload: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("unexpected client payload: %q", string(buf))
	}

	_ = client.Close()
	select {
	case err := <-proxyDone:
		if err != nil {
			t.Fatalf("proxy tcp: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("proxy tcp did not finish")
	}
	select {
	case err := <-originDone:
		if err != nil {
			t.Fatalf("origin: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("origin did not finish")
	}
}

type mockResponseWriter struct {
	header       http.Header
	status       int
	body         strings.Builder
	headerWrites int
}

func newMockResponseWriter() *mockResponseWriter {
	return &mockResponseWriter{header: make(http.Header)}
}

func (m *mockResponseWriter) Header() http.Header {
	return m.header
}

func (m *mockResponseWriter) Write(p []byte) (int, error) {
	return m.body.Write(p)
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.status = statusCode
}

func (m *mockResponseWriter) WriteRespHeaders(status int, header http.Header) error {
	m.headerWrites++
	m.status = status
	m.header = cloneHeader(header)
	return nil
}

func (m *mockResponseWriter) AddTrailer(trailerName, trailerValue string) {
	m.header.Add(trailerName, trailerValue)
}

func (m *mockResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if m.status == 0 {
		return nil, nil, errors.New("status not written before hijack")
	}
	return nil, nil, errors.New("hijack not supported")
}

type mockReadWriteAcker struct {
	net.Conn
	ackC chan struct{}
}

func newMockReadWriteAcker(conn net.Conn) *mockReadWriteAcker {
	return &mockReadWriteAcker{
		Conn: conn,
		ackC: make(chan struct{}),
	}
}

func (m *mockReadWriteAcker) AckConnection(_ string) error {
	close(m.ackC)
	return nil
}
