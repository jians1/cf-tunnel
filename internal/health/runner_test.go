package health

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRunnerStopsPromptlyWithOpenClientConnection(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	listen := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	runner := NewRunner(listen, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	waitForHealth(t, "http://"+listen+"/live")

	resp, err := http.Get("http://" + listen + "/live")
	if err != nil {
		t.Fatalf("open health request: %v", err)
	}
	defer resp.Body.Close()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runner run: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("health runner did not stop promptly after context cancellation")
	}
}

func waitForHealth(t *testing.T, url string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("health endpoint did not become ready: %s", url)
}
