package runtime

import (
	"bytes"
	"context"
	"errors"
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

func TestBridgeRunnerRunsConfiguredHTTP2HAConnectionsWithDistinctIndexes(t *testing.T) {
	t.Parallel()

	assertBridgeRunnerStartsHAConnections(t, "http2", func(options InstanceOptions) (uint8, ConnectedFuse) {
		return options.HTTP2.ConnIndex, options.HTTP2.ConnectedFuse
	})
}

func TestBridgeRunnerRunsConfiguredQUICHAConnectionsWithDistinctIndexes(t *testing.T) {
	t.Parallel()

	assertBridgeRunnerStartsHAConnections(t, "quic", func(options InstanceOptions) (uint8, ConnectedFuse) {
		return options.QUIC.ConnIndex, options.QUIC.ConnectedFuse
	})
}

func TestBridgeRunnerRunsHTTP2HAConnectionsWithStableConnectorID(t *testing.T) {
	t.Parallel()

	assertBridgeRunnerUsesStableConnectorID(t, "http2", func(options InstanceOptions) []byte {
		return options.HTTP2.ConnectorID
	})
}

func TestBridgeRunnerRunsQUICHAConnectionsWithStableConnectorID(t *testing.T) {
	t.Parallel()

	assertBridgeRunnerUsesStableConnectorID(t, "quic", func(options InstanceOptions) []byte {
		return options.QUIC.ConnectorID
	})
}

func TestBridgeRunnerWaitsForFirstConnectionBeforeStartingRemainingHAConnections(t *testing.T) {
	t.Parallel()

	session := testSession(t, "quic")
	session.HAConnections = 4
	runner := NewBridgeRunner(session, newDiscardSlogLogger())
	runner.registrationInterval = 0

	started := make(chan uint8, session.HAConnections)
	firstConnected := make(chan ConnectedFuse, 1)
	runner.instanceFactory = func(_ Session, _ *slog.Logger, options InstanceOptions) (bridgeInstance, error) {
		connIndex := options.QUIC.ConnIndex
		connectedFuse := options.QUIC.ConnectedFuse
		return bridgeInstanceFunc(func(ctx context.Context) error {
			started <- connIndex
			if connIndex == 0 {
				firstConnected <- connectedFuse
			}
			<-ctx.Done()
			return ctx.Err()
		}), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	select {
	case connIndex := <-started:
		if connIndex != 0 {
			t.Fatalf("expected first connection index 0, got %d", connIndex)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first HA connection to start")
	}

	select {
	case connIndex := <-started:
		t.Fatalf("connection %d started before first connection reported connected", connIndex)
	case <-time.After(50 * time.Millisecond):
	}

	var fuse ConnectedFuse
	select {
	case fuse = <-firstConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first connection fuse")
	}
	fuse.Connected()

	got := map[uint8]bool{0: true}
	for len(got) < session.HAConnections {
		select {
		case connIndex := <-started:
			got[connIndex] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for remaining HA connections; got indexes: %v", got)
		}
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected run error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for bridge runner to stop")
	}
}

func assertBridgeRunnerStartsHAConnections(t *testing.T, proto string, selectIndex func(InstanceOptions) (uint8, ConnectedFuse)) {
	t.Helper()

	session := testSession(t, proto)
	session.HAConnections = 4
	runner := NewBridgeRunner(session, newDiscardSlogLogger())
	runner.registrationInterval = 0

	started := make(chan uint8, session.HAConnections)
	runner.instanceFactory = func(_ Session, _ *slog.Logger, options InstanceOptions) (bridgeInstance, error) {
		connIndex, connectedFuse := selectIndex(options)
		return bridgeInstanceFunc(func(ctx context.Context) error {
			started <- connIndex
			if connIndex == 0 {
				connectedFuse.Connected()
			}
			<-ctx.Done()
			return ctx.Err()
		}), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	got := make(map[uint8]bool)
	for len(got) < session.HAConnections {
		select {
		case connIndex := <-started:
			got[connIndex] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for HA connections to start; got indexes: %v", got)
		}
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected run error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for bridge runner to stop")
	}

	for want := 0; want < session.HAConnections; want++ {
		if !got[uint8(want)] {
			t.Fatalf("missing connIndex %d; got indexes: %v", want, got)
		}
	}
}

func assertBridgeRunnerUsesStableConnectorID(t *testing.T, proto string, selectConnectorID func(InstanceOptions) []byte) {
	t.Helper()

	session := testSession(t, proto)
	session.HAConnections = 4
	runner := NewBridgeRunner(session, newDiscardSlogLogger())
	runner.registrationInterval = 0

	started := make(chan []byte, session.HAConnections)
	runner.instanceFactory = func(_ Session, _ *slog.Logger, options InstanceOptions) (bridgeInstance, error) {
		connIndex := options.QUIC.ConnIndex
		connectedFuse := options.QUIC.ConnectedFuse
		if proto == "http2" {
			connIndex = options.HTTP2.ConnIndex
			connectedFuse = options.HTTP2.ConnectedFuse
		}
		connectorID := append([]byte(nil), selectConnectorID(options)...)
		return bridgeInstanceFunc(func(ctx context.Context) error {
			started <- connectorID
			if connIndex == 0 {
				connectedFuse.Connected()
			}
			<-ctx.Done()
			return ctx.Err()
		}), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx)
	}()

	var first []byte
	for i := 0; i < session.HAConnections; i++ {
		select {
		case connectorID := <-started:
			if len(connectorID) != runtimeConnectorIDLength {
				t.Fatalf("expected %d-byte connector id, got %d bytes", runtimeConnectorIDLength, len(connectorID))
			}
			if i == 0 {
				first = connectorID
				continue
			}
			if !bytes.Equal(connectorID, first) {
				t.Fatalf("connector id changed: got %v want %v", connectorID, first)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for HA connections to start")
		}
	}

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected run error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for bridge runner to stop")
	}
}

type bridgeInstanceFunc func(context.Context) error

func (f bridgeInstanceFunc) Run(ctx context.Context) error {
	return f(ctx)
}

func TestBridgeRunnerOmitsPreparedDetailsAtInfo(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	runner := NewBridgeRunner(testSession(t, "quic"), logger)
	runner.SetQUICOptions(QUICRuntimeOptions{
		DialAddress: "not-a-udp-address",
	})

	if err := runner.Run(context.Background()); err == nil {
		t.Fatal("expected configured quic edge dial address to fail")
	}

	output := buf.String()
	if strings.Contains(output, "cftunnel runtime bridge prepared") {
		t.Fatalf("expected info logs to omit runtime preparation details; got %s", output)
	}
	if strings.Contains(output, "origin_insecure_skip_verify") {
		t.Fatalf("expected info logs to omit origin TLS details; got %s", output)
	}
}

func newDiscardSlogLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewBridgeRunnerAcceptsNilLogger(t *testing.T) {
	t.Parallel()

	runner := NewBridgeRunner(testSession(t, "http2"), nil)
	if runner == nil {
		t.Fatal("expected runner")
	}
	if runner.logger == nil {
		t.Fatal("expected default logger")
	}
}
