package runtime

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"

	cfdconnection "github.com/cloudflare/cloudflared/connection"
)

func TestHTTP2HarnessServeHTTP(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	prepared, err := PrepareRuntime(session)
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	log := newTestZeroLogger()
	observer := cfdconnection.NewObserver(&log, &log)
	_ = observer
	server, err := NewHTTP2ServerWithHandler(prepared, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream", "ok")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted"))
	}), HTTP2ServerOptions{})
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

	clientConn, err := NewHTTP2Client(server)
	if err != nil {
		t.Fatalf("new http2 client: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/demo", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := clientConn.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	headerPairs, err := cfdconnection.DeserializeHeaders(resp.Header.Get(cfdconnection.CanonicalResponseUserHeaders))
	if err != nil {
		t.Fatalf("deserialize response user headers: %v", err)
	}
	if got := findHeader(headerPairs, "X-Upstream"); got != "ok" {
		t.Fatalf("unexpected serialized header: %q", got)
	}
	if got := string(body); got != "accepted" {
		t.Fatalf("unexpected body: %q", got)
	}

	cancel()
	wg.Wait()
}

func TestHTTP2HarnessConfigurationGetter(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	prepared, err := PrepareRuntime(session)
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}
	upstreamOriginProxy := NewUpstreamOriginProxy(prepared.OriginProxy)
	orchestrator, err := NewUpstreamOrchestrator(upstreamOriginProxy, prepared.Session)
	if err != nil {
		t.Fatalf("new orchestrator: %v", err)
	}

	cfg, err := orchestrator.GetConfigJSON()
	if err != nil {
		t.Fatalf("get config json: %v", err)
	}
	if len(cfg) == 0 {
		t.Fatal("expected non-empty config json")
	}
}

func findHeader(headers []cfdconnection.HTTPHeader, name string) string {
	for _, h := range headers {
		if h.Name == name {
			return h.Value
		}
	}
	return ""
}
