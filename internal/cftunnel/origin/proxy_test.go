package origin

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestProxyForwardsHTTP(t *testing.T) {
	t.Parallel()

	upstream := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "ok")
		w.Header().Set("X-Seen-Host", r.Host)
		_, _ = w.Write([]byte("proxied"))
	}))
	defer upstream.Close()

	target := Target{
		Raw:      upstream.URL,
		Protocol: ProtocolHTTP,
		URL:      MustParseURL(upstream.URL),
	}
	proxy := NewProxy(target)
	server := startLocalHTTPServer(t, proxy.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("get through proxy: %v", err)
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
		t.Fatalf("unexpected upstream header: %q", got)
	}
}

func TestProxyOverridesHostAndSetsForwardedHost(t *testing.T) {
	t.Parallel()

	upstream := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Host", r.Host)
		w.Header().Set("X-Forwarded-Host", r.Header.Get("X-Forwarded-Host"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	target := Target{
		Raw:        upstream.URL,
		Protocol:   ProtocolHTTP,
		URL:        MustParseURL(upstream.URL),
		ServerName: "origin.example.com",
	}
	proxy := NewProxy(target)
	server := startLocalHTTPServer(t, proxy.Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = "incoming.example.net"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Seen-Host"); got != "origin.example.com" {
		t.Fatalf("unexpected seen host: %q", got)
	}
	if got := resp.Header.Get("X-Forwarded-Host"); got != "incoming.example.net" {
		t.Fatalf("unexpected forwarded host: %q", got)
	}
}

func TestProxySupportsInsecureHTTPSOrigin(t *testing.T) {
	t.Parallel()

	upstream := startLocalTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("secure"))
	}))
	defer upstream.Close()

	target := Target{
		Raw:                upstream.URL,
		Protocol:           ProtocolHTTPS,
		URL:                MustParseURL(upstream.URL),
		InsecureSkipVerify: true,
	}
	proxy := NewProxy(target)
	server := startLocalHTTPServer(t, proxy.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("get through https proxy: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := string(body); got != "secure" {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestProxyPreservesWebsocketUpgradeHeaders(t *testing.T) {
	t.Parallel()

	upstream := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Connection"), "Upgrade") {
			t.Fatalf("unexpected connection header: %q", r.Header.Get("Connection"))
		}
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			t.Fatalf("unexpected upgrade header: %q", r.Header.Get("Upgrade"))
		}
		w.Header().Set("Connection", "Upgrade")
		w.Header().Set("Upgrade", "websocket")
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	defer upstream.Close()

	target := Target{
		Raw:                  upstream.URL,
		Protocol:             ProtocolWS,
		URL:                  MustParseURL(upstream.URL),
		WebsocketUpgradeMode: true,
	}
	proxy := NewProxy(target)
	server := startLocalHTTPServer(t, proxy.Handler())
	defer server.Close()

	conn, err := net.DialTimeout("tcp", trimScheme(server.URL), 5*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()

	req := "GET /chat HTTP/1.1\r\n" +
		"Host: example.test\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: websocket\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestNewTransportUsesConnectionPoolDefaults(t *testing.T) {
	t.Parallel()

	transport := newTransport(Target{})

	if transport.MaxIdleConns != 256 {
		t.Fatalf("unexpected max idle conns: %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 64 {
		t.Fatalf("unexpected max idle conns per host: %d", transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost != 256 {
		t.Fatalf("unexpected max conns per host: %d", transport.MaxConnsPerHost)
	}
}

func TestWebsocketCopyBufferPoolProvidesExpectedBuffer(t *testing.T) {
	t.Parallel()

	buf := getWebsocketCopyBuffer()
	if len(buf) != websocketCopyBufferSize {
		t.Fatalf("unexpected copy buffer size: %d", len(buf))
	}
	putWebsocketCopyBuffer(buf)
}

func TestCopyAndCloseCopiesWithBufferPool(t *testing.T) {
	t.Parallel()

	dst := &recordingWriteCloser{}
	errC := make(chan error, 1)

	copyAndClose(errC, dst, strings.NewReader("payload"))

	if err := <-errC; err != nil {
		t.Fatalf("copy and close: %v", err)
	}
	if got := dst.String(); got != "payload" {
		t.Fatalf("unexpected copied payload: %q", got)
	}
	if !dst.closed {
		t.Fatal("expected destination to be closed")
	}
}

func TestCopyAndCloseUsesCloseWriteWhenAvailable(t *testing.T) {
	t.Parallel()

	dst := &recordingHalfWriteCloser{}
	errC := make(chan error, 1)

	copyAndClose(errC, dst, strings.NewReader("payload"))

	if err := <-errC; err != nil {
		t.Fatalf("copy and close: %v", err)
	}
	if got := dst.String(); got != "payload" {
		t.Fatalf("unexpected copied payload: %q", got)
	}
	if !dst.closedWrite {
		t.Fatal("expected destination write side to be closed")
	}
	if dst.closed {
		t.Fatal("did not expect full close when close write is available")
	}
}

func TestProxyWebsocketStreamsWaitsForBothDirections(t *testing.T) {
	t.Parallel()

	upstream := &recordingHalfDuplexConn{
		readC: make(chan []byte, 1),
		doneC: make(chan struct{}),
	}
	var downstream bytes.Buffer

	errC := make(chan error, 1)
	go func() {
		errC <- proxyWebsocketStreams(&downstream, upstream, strings.NewReader("request"))
	}()

	select {
	case err := <-errC:
		t.Fatalf("proxy returned before upstream response finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if got := upstream.String(); got != "request" {
		t.Fatalf("unexpected upstream request payload: %q", got)
	}
	if !upstream.closedWrite {
		t.Fatal("expected upstream write side to be closed")
	}

	upstream.readC <- []byte("response")
	close(upstream.doneC)

	select {
	case err := <-errC:
		if err != nil {
			t.Fatalf("proxy websocket streams: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not return after both directions finished")
	}
	if got := downstream.String(); got != "response" {
		t.Fatalf("unexpected downstream response payload: %q", got)
	}
}

func TestCopyWithWebsocketBufferCopiesPayload(t *testing.T) {
	t.Parallel()

	var dst bytes.Buffer

	n, err := copyWithWebsocketBuffer(&dst, strings.NewReader("payload"))
	if err != nil {
		t.Fatalf("copy with websocket buffer: %v", err)
	}
	if n != int64(len("payload")) {
		t.Fatalf("unexpected copied bytes: %d", n)
	}
	if got := dst.String(); got != "payload" {
		t.Fatalf("unexpected copied payload: %q", got)
	}
}

type localHTTPServer struct {
	ln     net.Listener
	server *http.Server
	URL    string
}

func (s *localHTTPServer) Close() {
	_ = s.server.Close()
	_ = s.ln.Close()
}

func startLocalHTTPServer(t *testing.T, handler http.Handler) *localHTTPServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen http: %v", err)
	}

	srv := &http.Server{Handler: handler}
	go func() {
		_ = srv.Serve(ln)
	}()

	return &localHTTPServer{
		ln:     ln,
		server: srv,
		URL:    "http://" + ln.Addr().String(),
	}
}

func startLocalTLSServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	return httptest.NewTLSServer(handler)
}

func trimScheme(raw string) string {
	return strings.TrimPrefix(strings.TrimPrefix(raw, "http://"), "https://")
}

func MustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

type recordingWriteCloser struct {
	bytes.Buffer
	closed bool
}

func (w *recordingWriteCloser) Close() error {
	w.closed = true
	return nil
}

type recordingHalfWriteCloser struct {
	recordingWriteCloser
	closedWrite bool
}

func (w *recordingHalfWriteCloser) CloseWrite() error {
	w.closedWrite = true
	return nil
}

type recordingHalfDuplexConn struct {
	writeBuf    bytes.Buffer
	closedWrite bool
	readC       chan []byte
	doneC       chan struct{}
}

func (c *recordingHalfDuplexConn) String() string {
	return c.writeBuf.String()
}

func (c *recordingHalfDuplexConn) Write(p []byte) (int, error) {
	return c.writeBuf.Write(p)
}

func (c *recordingHalfDuplexConn) Read(p []byte) (int, error) {
	select {
	case payload := <-c.readC:
		return copy(p, payload), nil
	case <-c.doneC:
		return 0, io.EOF
	}
}

func (c *recordingHalfDuplexConn) Close() error {
	return nil
}

func (c *recordingHalfDuplexConn) CloseWrite() error {
	c.closedWrite = true
	return nil
}
