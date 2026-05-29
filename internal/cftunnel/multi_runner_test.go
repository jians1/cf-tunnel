package cftunnel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

func TestNewMultiRunnerRejectsEmptyTunnels(t *testing.T) {
	t.Parallel()

	_, err := NewMultiRunner(nil, testLogger())
	if err == nil {
		t.Fatal("expected error for empty tunnel set")
	}
}

func TestNewMultiRunnerBuildsNamedRunners(t *testing.T) {
	t.Parallel()

	multi, err := NewMultiRunner([]config.NamedTunnelConfig{
		{
			Name: "alpha",
			CFTunnel: config.CFTunnelConfig{
				EdgeProtocol: config.EdgeProtocolHTTP2,
				Target:       "http://127.0.0.1:8081",
			},
		},
		{
			Name: "beta",
			CFTunnel: config.CFTunnelConfig{
				EdgeProtocol: config.EdgeProtocolHTTP2,
				Target:       "http://127.0.0.1:8082",
			},
		},
	}, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(multi.runners) != 2 {
		t.Fatalf("unexpected runner count: %d", len(multi.runners))
	}
	if multi.runners[0].Name() != "alpha" {
		t.Fatalf("unexpected first runner name: %s", multi.runners[0].Name())
	}
	if multi.runners[1].Name() != "beta" {
		t.Fatalf("unexpected second runner name: %s", multi.runners[1].Name())
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMultiRunnerRunRejectsNoRunners(t *testing.T) {
	t.Parallel()

	multi := &MultiRunner{logger: testLogger()}
	if err := multi.Run(context.Background()); err == nil {
		t.Fatal("expected run error")
	}
}

func TestJoinTunnelErrors(t *testing.T) {
	t.Parallel()

	single := errors.New("x")
	if got := joinTunnelErrors([]error{single}); got != single {
		t.Fatalf("expected single error passthrough, got %v", got)
	}

	got := joinTunnelErrors([]error{errors.New("a"), errors.New("b")})
	if got == nil {
		t.Fatal("expected joined error")
	}
	if !strings.Contains(got.Error(), "multiple tunnel failures (2)") {
		t.Fatalf("unexpected joined error header: %v", got)
	}
	if !strings.Contains(got.Error(), "a") || !strings.Contains(got.Error(), "b") {
		t.Fatalf("unexpected joined error body: %v", got)
	}
}

func TestMultiRunnerReadinessSummary(t *testing.T) {
	t.Parallel()

	multi := &MultiRunner{
		runners: []tunnelRunner{
			fakeTunnelRunner{name: "a", run: func(context.Context) error { return nil }},
			fakeTunnelRunner{name: "b", run: func(context.Context) error { return nil }},
		},
		state: map[string]string{
			"a": "starting",
			"b": "failed",
		},
	}

	got := multi.ReadinessSummary()
	if !strings.Contains(got, "mode=multi total=2 running=1 failed=1") {
		t.Fatalf("unexpected summary header: %s", got)
	}
	if !strings.Contains(got, "a:starting") || !strings.Contains(got, "b:failed") {
		t.Fatalf("unexpected summary details: %s", got)
	}
}

type fakeTunnelRunner struct {
	name string
	run  func(context.Context) error
}

func (r fakeTunnelRunner) Name() string { return r.name }
func (r fakeTunnelRunner) Run(ctx context.Context) error {
	return r.run(ctx)
}

func TestMultiRunnerIntegrationTwoTunnelsActiveThenCanceled(t *testing.T) {
	t.Parallel()

	started := make(chan string, 2)
	stop := make(chan struct{})

	makeBlocking := func(name string) fakeTunnelRunner {
		return fakeTunnelRunner{
			name: name,
			run: func(ctx context.Context) error {
				started <- name
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-stop:
					return nil
				}
			},
		}
	}

	multi := &MultiRunner{
		runners: []tunnelRunner{
			makeBlocking("t1"),
			makeBlocking("t2"),
		},
		logger: testLogger(),
		state:  map[string]string{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- multi.Run(ctx) }()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("tunnel did not start in time")
		}
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected clean shutdown, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("multi runner did not stop after context cancel")
	}
}

func TestMultiRunnerIntegrationFailFastAndAggregate(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 2)
	var mu sync.Mutex
	var canceled int

	multi := &MultiRunner{
		runners: []tunnelRunner{
			fakeTunnelRunner{
				name: "bad",
				run: func(context.Context) error {
					started <- struct{}{}
					return errors.New("boom")
				},
			},
			fakeTunnelRunner{
				name: "peer",
				run: func(ctx context.Context) error {
					started <- struct{}{}
					<-ctx.Done()
					mu.Lock()
					canceled++
					mu.Unlock()
					return ctx.Err()
				},
			},
		},
		logger: testLogger(),
		state:  map[string]string{},
	}

	done := make(chan error, 1)
	go func() { done <- multi.Run(context.Background()) }()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(300 * time.Millisecond):
			t.Fatal("runner did not start in time")
		}
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected fail-fast error")
		}
		if !strings.Contains(err.Error(), "tunnel bad: boom") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("multi runner did not fail fast")
	}

	mu.Lock()
	defer mu.Unlock()
	if canceled == 0 {
		t.Fatal("expected peer tunnel to be canceled on fail-fast")
	}
}
