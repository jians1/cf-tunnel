package runtime

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

func TestPipeHTTP2TransportFactory(t *testing.T) {
	t.Parallel()

	factory := PipeHTTP2TransportFactory{}
	transport, err := factory.NewTransport(context.Background())
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	defer transport.Close()
	if transport.EdgeConn() == nil || transport.ServerConn() == nil {
		t.Fatal("expected edge and server conns")
	}
}

func TestLoopbackHTTP2TransportFactory(t *testing.T) {
	t.Parallel()

	factory := LoopbackHTTP2TransportFactory{}
	transport, err := factory.NewTransport(context.Background())
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	defer transport.Close()
	if transport.EdgeConn() == nil || transport.ServerConn() == nil {
		t.Fatal("expected edge and server conns")
	}
}

func TestDialHTTP2TransportFactoryRejectsEmptyAddress(t *testing.T) {
	t.Parallel()

	factory := DialHTTP2TransportFactory{}
	if _, err := factory.NewTransport(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestHTTP2ServerAcceptsDialTransportFactory(t *testing.T) {
	t.Parallel()

	edge := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer edge.Close()

	session := testSession(t, "http2")
	prepared, err := PrepareRuntime(session)
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	_, err = NewHTTP2ServerWithHandler(prepared, nil, http.NotFoundHandler(), HTTP2ServerOptions{
		TransportFactory: DialHTTP2TransportFactory{
			Address:   mustHostPort(t, edge.URL),
			TLSConfig: &tls.Config{InsecureSkipVerify: true}, // loopback test server only
		},
	})
	if err != nil {
		t.Fatalf("new http2 server with dial transport: %v", err)
	}
}

func mustHostPort(t *testing.T, raw string) string {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u.Host
}

func TestHTTP2ServerWithLoopbackTransport(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	prepared, err := PrepareRuntime(session)
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	server, err := NewHTTP2ServerWithHandler(prepared, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}), HTTP2ServerOptions{
		TransportFactory: LoopbackHTTP2TransportFactory{},
	})
	if err != nil {
		t.Fatalf("new http2 server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = server.Serve(ctx)
	}()

	client, err := NewHTTP2Client(server)
	if err != nil {
		t.Fatalf("new http2 client: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/loopback", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := client.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("unexpected body: %q", string(body))
	}

	cancel()
	wg.Wait()
}
