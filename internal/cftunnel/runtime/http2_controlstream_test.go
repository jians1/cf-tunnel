package runtime

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestHTTP2ServerControlStreamRegistrationLifecycle(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	prepared, err := PrepareRuntime(session)
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	rpcClientFactory := mockRPCClientFactory{
		registered:   make(chan struct{}),
		unregistered: make(chan struct{}),
	}
	adapter := NewUpstreamAdapter()
	binding, err := adapter.Bind(session)
	if err != nil {
		t.Fatalf("bind upstream runtime: %v", err)
	}
	controlStream := NewControlStream(runtimeControlStreamOptions{
		ConnectedFuse:      mockConnectedFuse{},
		TunnelProperties:   binding.TunnelProperties,
		ConnIndex:          0,
		EdgeAddress:        nil,
		RegisterClientFunc: rpcClientFactory.newMockRPCClient,
		RegisterTimeout:    1 * time.Second,
		GracefulShutdownC:  nil,
		GracePeriod:        1 * time.Second,
	})

	server, err := NewHTTP2ServerWithHandler(prepared, nil, http.NotFoundHandler(), HTTP2ServerOptions{
		ControlStreamHandler: controlStream,
		ConnectedFuse:        mockConnectedFuse{},
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(InternalUpgradeHeader, ControlStreamUpgrade)

	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := client.RoundTrip(req)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
		}
	}()

	select {
	case <-rpcClientFactory.registered:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for registration")
	}

	cancel()

	select {
	case <-rpcClientFactory.unregistered:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for unregistration")
	}

	wg.Wait()
}

func TestHTTP2ServerBuildsControlStreamFromOptions(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	instance, err := NewInstanceWithOptions(session, nil, HTTP2ServerOptions{
		RegistrationClientFunc: (&mockRPCClientFactory{
			registered:   make(chan struct{}),
			unregistered: make(chan struct{}),
		}).newMockRPCClient,
		ConnectedFuse:   mockConnectedFuse{},
		RegisterTimeout: time.Second,
		GracePeriod:     time.Second,
	})
	if err != nil {
		t.Fatalf("new instance with options: %v", err)
	}
	if instance.HTTP2Server == nil {
		t.Fatal("expected http2 server")
	}
}

func TestHTTP2ServerOptionsDefaultRegistrationFactory(t *testing.T) {
	t.Parallel()

	opts := (HTTP2ServerOptions{}).withDefaults()
	if opts.RegistrationClientFunc == nil {
		t.Fatal("expected default registration client factory")
	}
	if opts.TransportFactory == nil {
		t.Fatal("expected default transport factory")
	}
}

type mockConnectedFuse struct{}

func (mockConnectedFuse) Connected()        {}
func (mockConnectedFuse) IsConnected() bool { return true }
