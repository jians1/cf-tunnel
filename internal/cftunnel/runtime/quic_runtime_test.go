package runtime

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestNoopDatagramSessionHandler(t *testing.T) {
	t.Parallel()

	handler := newNoopDatagramSessionHandler()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- handler.Serve(ctx)
	}()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("serve did not return after context cancellation")
	}

	if _, err := handler.RegisterUdpSession(context.Background(), uuid.New(), net.IPv4(127, 0, 0, 1), 53, time.Second, ""); err == nil {
		t.Fatal("expected register udp session to fail")
	}
	if err := handler.UnregisterUdpSession(context.Background(), uuid.New(), "done"); err == nil {
		t.Fatal("expected unregister udp session to fail")
	}
}

func TestQUICRuntimeUsesRuntimeTunnelConnection(t *testing.T) {
	t.Parallel()

	runtime := &QUICRuntime{
		tunnelConn: &RuntimeQUICConnection{},
	}

	if _, ok := runtime.tunnelConn.(*RuntimeQUICConnection); !ok {
		t.Fatalf("expected runtime quic connection, got %T", runtime.tunnelConn)
	}
}

func TestNewQUICConfigSetsExplicitFlowControlWindows(t *testing.T) {
	t.Parallel()

	cfg := newQUICConfig(false)

	if cfg.MaxConnectionReceiveWindow != defaultQUICConnectionReceiveWindow {
		t.Fatalf("unexpected connection receive window: got %d want %d", cfg.MaxConnectionReceiveWindow, defaultQUICConnectionReceiveWindow)
	}
	if cfg.MaxStreamReceiveWindow != defaultQUICStreamReceiveWindow {
		t.Fatalf("unexpected stream receive window: got %d want %d", cfg.MaxStreamReceiveWindow, defaultQUICStreamReceiveWindow)
	}
}

func TestNewQUICConfigUsesCloudflaredRuntimeTimeouts(t *testing.T) {
	t.Parallel()

	cfg := newQUICConfig(true)

	if cfg.HandshakeIdleTimeout != 5*time.Second {
		t.Fatalf("unexpected handshake idle timeout: got %s", cfg.HandshakeIdleTimeout)
	}
	if cfg.MaxIdleTimeout != 5*time.Second {
		t.Fatalf("unexpected max idle timeout: got %s", cfg.MaxIdleTimeout)
	}
	if cfg.KeepAlivePeriod != time.Second {
		t.Fatalf("unexpected keepalive period: got %s", cfg.KeepAlivePeriod)
	}
	if cfg.MaxIncomingStreams != 1<<60 {
		t.Fatalf("unexpected max incoming streams: got %d", cfg.MaxIncomingStreams)
	}
	if cfg.MaxIncomingUniStreams != 1<<60 {
		t.Fatalf("unexpected max incoming unidirectional streams: got %d", cfg.MaxIncomingUniStreams)
	}
	if !cfg.EnableDatagrams {
		t.Fatal("expected datagrams to remain enabled for edge compatibility")
	}
	if cfg.InitialPacketSize != 1232 {
		t.Fatalf("unexpected IPv4 initial packet size: got %d", cfg.InitialPacketSize)
	}
}

func TestNewQUICConfigUsesIPv6InitialPacketSize(t *testing.T) {
	t.Parallel()

	cfg := newQUICConfig(false)

	if cfg.InitialPacketSize != 1252 {
		t.Fatalf("unexpected IPv6 initial packet size: got %d", cfg.InitialPacketSize)
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
