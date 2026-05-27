package runtime

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
)

func TestHTTP2ClientRoundTrip(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	prepared, err := PrepareRuntime(session)
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	server, err := NewHTTP2ServerWithHandler(prepared, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
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

	client, err := NewHTTP2Client(server)
	if err != nil {
		t.Fatalf("new http2 client: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/demo", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	cancel()
	wg.Wait()
}
