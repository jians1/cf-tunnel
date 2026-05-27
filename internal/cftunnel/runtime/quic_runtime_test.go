package runtime

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	quicgo "github.com/quic-go/quic-go"
)

func TestQUICRuntimeRunReturnsWhenContextEndsEvenIfTunnelServeBlocks(t *testing.T) {
	t.Parallel()

	runtime := &QUICRuntime{
		tunnelConn: blockingTunnelConnection{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runtime.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context canceled, got: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("quic runtime did not return after context cancellation")
	}
}

type blockingTunnelConnection struct{}

func (blockingTunnelConnection) Serve(context.Context) error {
	select {}
}

func TestNewQUICRuntimeWithOptionsRequiresRealDialConfig(t *testing.T) {
	t.Parallel()

	runtime, err := NewQUICRuntimeWithOptions(testSession(t, "quic"), slog.Default(), QUICRuntimeOptions{
		DialAddress: "not-a-udp-address",
	})
	if err == nil {
		_ = runtime.Close()
		t.Fatal("expected quic runtime to use the configured edge dial address")
	}
	if !errors.Is(err, errMissingQUICDialConfig) {
		t.Fatalf("expected missing dial config error, got: %v", err)
	}
}

func TestCloseQUICRuntimeStartupResourcesClosesAllResources(t *testing.T) {
	t.Parallel()

	conn := &recordingQUICConnectionCloser{}
	listener := &recordingCloser{}
	udpConn := &recordingCloser{err: errors.New("udp close failed")}

	err := closeQUICRuntimeStartupResources(conn, listener, udpConn)

	if !conn.closed {
		t.Fatal("expected quic connection to be closed")
	}
	if !listener.closed {
		t.Fatal("expected quic listener to be closed")
	}
	if !udpConn.closed {
		t.Fatal("expected udp connection to be closed")
	}
	if err == nil || err.Error() != "udp close failed" {
		t.Fatalf("expected udp close error, got: %v", err)
	}
}

func TestQUICRuntimeCloseClosesClientQUICConnection(t *testing.T) {
	t.Parallel()

	conn := &recordingQUICConnectionCloser{}
	runtime := &QUICRuntime{
		quicConn: conn,
	}

	err := runtime.Close()
	if err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	if !conn.closed {
		t.Fatal("expected client quic connection to be closed")
	}
}

type recordingQUICConnectionCloser struct {
	closed bool
}

func (c *recordingQUICConnectionCloser) CloseWithError(quicgo.ApplicationErrorCode, string) error {
	c.closed = true
	return nil
}

type recordingCloser struct {
	closed bool
	err    error
}

func (c *recordingCloser) Close() error {
	c.closed = true
	return c.err
}
