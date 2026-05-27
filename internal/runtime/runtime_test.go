package runtime

import (
	"context"
	"io"
	"log/slog"
	"strings"
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

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
