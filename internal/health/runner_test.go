package health

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRunnerReadyStatusWithoutProviderIsNotReady(t *testing.T) {
	t.Parallel()

	runner := NewRunner(":9090", slog.New(slog.NewTextHandler(io.Discard, nil)))
	status := runner.ReadyStatus()
	if status.Ready {
		t.Fatalf("expected not ready, got %#v", status)
	}
	if status.Summary != "not ready" {
		t.Fatalf("unexpected summary %q", status.Summary)
	}
}

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

func TestRunnerShutdownTimesOutWhenReadinessHandlerBlocks(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	listen := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	runner := NewRunner(listen, slog.New(slog.NewTextHandler(io.Discard, nil)))
	runner.SetReadyProvider(func() ReadyStatus {
		close(providerStarted)
		<-releaseProvider
		return ReadyStatus{Ready: true, Summary: "ready"}
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	waitForHealthStatus(t, "http://"+listen+"/live", http.StatusOK)
	requestDone := make(chan struct{})
	go func() {
		resp, err := http.Get("http://" + listen + "/ready")
		if err == nil {
			_ = resp.Body.Close()
		}
		close(requestDone)
	}()

	select {
	case <-providerStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("readiness provider did not start")
	}

	cancel()
	defer close(releaseProvider)

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected shutdown timeout error")
		}
		if !strings.Contains(err.Error(), "context deadline exceeded") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("health runner did not return after shutdown timeout")
	}

	select {
	case <-requestDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocked readiness request did not finish after provider release")
	}
}

func TestRunnerReadyUsesSummaryProvider(t *testing.T) {
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
	runner.SetReadySummaryProvider(func() string {
		return "mode=multi total=2 running=2 failed=0"
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	waitForHealth(t, "http://"+listen+"/ready")

	resp, err := http.Get("http://" + listen + "/ready")
	if err != nil {
		t.Fatalf("get ready: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read ready body: %v", err)
	}
	if !strings.Contains(string(body), "mode=multi total=2") {
		t.Fatalf("unexpected ready body: %s", string(body))
	}

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

func TestRunnerReadyReturnsServiceUnavailableWhenProviderNotReady(t *testing.T) {
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
	runner.SetReadyProvider(func() ReadyStatus {
		return ReadyStatus{Ready: false, Summary: "mode=single total=1 ready=0 failed=0 details=[cftunnel:starting]"}
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("runner run: %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("health runner did not stop")
		}
	}()

	waitForHealthStatus(t, "http://"+listen+"/live", http.StatusOK)
	resp, err := http.Get("http://" + listen + "/ready")
	if err != nil {
		t.Fatalf("get ready: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read ready body: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected ready status: %d body=%s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "cftunnel:starting") {
		t.Fatalf("unexpected ready body: %s", string(body))
	}
}

func TestRunnerReadyReturnsOKWhenProviderReady(t *testing.T) {
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
	runner.SetReadyProvider(func() ReadyStatus {
		return ReadyStatus{Ready: true, Summary: "mode=single total=1 ready=1 failed=0 details=[cftunnel:ready]"}
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("runner run: %v", err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("health runner did not stop")
		}
	}()

	waitForHealthStatus(t, "http://"+listen+"/ready", http.StatusOK)
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

func waitForHealthStatus(t *testing.T, url string, status int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == status {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("health endpoint did not return %d: %s", status, url)
}
