package runtime

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestBridgeRunnerHTTP2UsesRuntimeTransport(t *testing.T) {
	t.Parallel()

	runner := NewBridgeRunner(testSession(t, "http2"), newDiscardSlogLogger())
	runner.SetHTTP2Options(HTTP2ServerOptions{
		DialAddress:      "198.51.100.10:443",
		DialTimeout:      50 * time.Millisecond,
		TransportFactory: PipeHTTP2TransportFactory{},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := runner.Run(ctx)
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "dial http2 transport") {
		t.Fatalf("runner ignored configured transport factory: %v", err)
	}
}

func TestBridgeRunnerHTTP2LocalEdgeDriverStaysAliveUntilContextEnds(t *testing.T) {
	t.Parallel()

	runner := NewBridgeRunner(testSession(t, "http2"), newDiscardSlogLogger())
	runner.SetHTTP2Options(HTTP2ServerOptions{
		LocalEdgeDriver: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := runner.Run(ctx)
	if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
		if strings.Contains(err.Error(), "connection with edge closed") {
			t.Fatalf("local edge driver did not keep http2 runtime alive long enough: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBridgeRunnerQUICUsesConfiguredEdgeDial(t *testing.T) {
	t.Parallel()

	runner := NewBridgeRunner(testSession(t, "quic"), newDiscardSlogLogger())
	runner.SetQUICOptions(QUICRuntimeOptions{
		DialAddress: "not-a-udp-address",
	})

	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("expected configured quic edge dial address to be used")
	}
	if !strings.Contains(err.Error(), "parse quic dial address") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newDiscardSlogLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
