package runtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunWithOptionsReturnsShutdownTimeoutWhenRunnerIgnoresContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	runner := blockingRunner{started: make(chan struct{})}
	done := make(chan error, 1)

	go func() {
		done <- RunWithOptions(ctx, discardLogger(), Options{
			ShutdownTimeout: 20 * time.Millisecond,
		}, runner)
	}()

	select {
	case <-runner.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runner did not start")
	}

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected shutdown timeout error")
		}
		if !strings.Contains(err.Error(), "shutdown timeout") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runtime did not return after shutdown timeout")
	}
}

func TestRunWithOptionsReturnsRunnerErrorWithNameAndCancelsPeers(t *testing.T) {
	t.Parallel()

	var canceled atomic.Bool
	err := RunWithOptions(context.Background(), discardLogger(), Options{}, failingRunner{
		name: "bad",
		err:  errors.New("boom"),
	}, blockingCtxRunner{
		name: "peer",
		onCancel: func() {
			canceled.Store(true)
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bad: boom") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !canceled.Load() {
		t.Fatal("expected peer runner canceled after failure")
	}
}

type blockingRunner struct {
	started chan struct{}
}

func (r blockingRunner) Name() string {
	return "blocking"
}

func (r blockingRunner) Run(context.Context) error {
	close(r.started)
	select {}
}

type failingRunner struct {
	name string
	err  error
}

func (r failingRunner) Name() string { return r.name }
func (r failingRunner) Run(context.Context) error {
	return r.err
}

type blockingCtxRunner struct {
	name     string
	onCancel func()
}

func (r blockingCtxRunner) Name() string { return r.name }
func (r blockingCtxRunner) Run(ctx context.Context) error {
	<-ctx.Done()
	if r.onCancel != nil {
		r.onCancel()
	}
	return ctx.Err()
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
