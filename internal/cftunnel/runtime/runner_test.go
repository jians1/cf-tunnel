package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
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

func TestBridgeRunnerUsesConnectionIndexedEdgeProviders(t *testing.T) {
	t.Parallel()

	runner := NewBridgeRunner(testSession(t, "http2"), newDiscardSlogLogger())
	runner.SetHTTP2Options(HTTP2ServerOptions{
		EdgeAddressProvider: indexedEdgeAddressProvider{},
	})
	runner.SetQUICOptions(QUICRuntimeOptions{
		EdgeAddressProvider: indexedEdgeAddressProvider{},
	})

	options := runner.instanceOptions(2)

	http2Address, err := options.HTTP2.EdgeAddressProvider.ResolveHTTP2Address()
	if err != nil {
		t.Fatalf("resolve http2 address: %v", err)
	}
	quicAddress, err := options.QUIC.EdgeAddressProvider.ResolveQUICAddress()
	if err != nil {
		t.Fatalf("resolve quic address: %v", err)
	}

	if http2Address != "http2-2.example.com:443" {
		t.Fatalf("unexpected http2 address: %s", http2Address)
	}
	if quicAddress != "quic-2.example.com:7844" {
		t.Fatalf("unexpected quic address: %s", quicAddress)
	}
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

type indexedEdgeAddressProvider struct {
	index uint8
}

func (p indexedEdgeAddressProvider) ForConnIndex(connIndex uint8) EdgeAddressProvider {
	return indexedEdgeAddressProvider{index: connIndex}
}

func (p indexedEdgeAddressProvider) ResolveHTTP2Address() (string, error) {
	return "http2-" + strconv.Itoa(int(p.index)) + ".example.com:443", nil
}

func (p indexedEdgeAddressProvider) ResolveQUICAddress() (string, error) {
	return "quic-" + strconv.Itoa(int(p.index)) + ".example.com:7844", nil
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

func TestBridgeRunnerRetriesConnectionThenSucceeds(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	session.HAConnections = 1
	runner := NewBridgeRunner(session, newDiscardSlogLogger())
	runner.backoffBase = time.Millisecond
	runner.backoffCap = 2 * time.Millisecond

	var attempts int32
	runner.instanceFactory = func(_ Session, _ *slog.Logger, _ InstanceOptions) (bridgeInstance, error) {
		return bridgeInstanceFunc(func(ctx context.Context) error {
			n := atomic.AddInt32(&attempts, 1)
			if n < 3 {
				return errors.New("connection with edge closed")
			}
			<-ctx.Done()
			return ctx.Err()
		}), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()

	// Wait until the third attempt (the one that stays connected) is running.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&attempts) < 3 {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for retries; attempts=%d", atomic.LoadInt32(&attempts))
		case err := <-done:
			t.Fatalf("runner exited early: %v", err)
		default:
			time.Sleep(time.Millisecond)
		}
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected run error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for runner to stop")
	}
}

func TestBridgeRunnerStopsAfterMaxRetries(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	session.HAConnections = 1
	runner := NewBridgeRunner(session, newDiscardSlogLogger())
	runner.maxConnRetries = 3
	runner.backoffBase = time.Millisecond
	runner.backoffCap = 2 * time.Millisecond

	var attempts int32
	runner.instanceFactory = func(_ Session, _ *slog.Logger, _ InstanceOptions) (bridgeInstance, error) {
		return bridgeInstanceFunc(func(context.Context) error {
			atomic.AddInt32(&attempts, 1)
			return errors.New("EDUPCONN")
		}), nil
	}

	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if !strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("unexpected error: %v", err)
	}
	// initial attempt + maxConnRetries retries = 4
	if got := atomic.LoadInt32(&attempts); got != 4 {
		t.Fatalf("expected 4 attempts, got %d", got)
	}
}

func TestBridgeRunnerRetryResetsAfterSuccessfulConnect(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	session.HAConnections = 1
	runner := NewBridgeRunner(session, newDiscardSlogLogger())
	runner.maxConnRetries = 2
	runner.backoffBase = time.Millisecond
	runner.backoffCap = 2 * time.Millisecond

	var attempts int32
	runner.instanceFactory = func(_ Session, _ *slog.Logger, options InstanceOptions) (bridgeInstance, error) {
		fuse := options.HTTP2.ConnectedFuse
		return bridgeInstanceFunc(func(context.Context) error {
			n := atomic.AddInt32(&attempts, 1)
			// Every attempt connects (resetting the budget) then drops, so the
			// runner would loop forever if the reset works. Stop it after a
			// handful of attempts to prove it never hit the retry ceiling.
			if fuse != nil {
				fuse.Connected()
			}
			if n >= 6 {
				return &nonRetryableError{err: errors.New("stop")}
			}
			return errors.New("connection with edge closed")
		}), nil
	}

	err := runner.Run(context.Background())
	if err == nil || strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("expected non-exhaustion stop error, got: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got < 6 {
		t.Fatalf("expected budget to reset across connects; attempts=%d", got)
	}
}

func TestBridgeRunnerRetryCanceledDuringBackoff(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	session.HAConnections = 1
	runner := NewBridgeRunner(session, newDiscardSlogLogger())
	runner.maxConnRetries = 100
	runner.backoffBase = time.Hour
	runner.backoffCap = time.Hour

	runner.instanceFactory = func(_ Session, _ *slog.Logger, _ InstanceOptions) (bridgeInstance, error) {
		return bridgeInstanceFunc(func(context.Context) error {
			return errors.New("connection with edge closed")
		}), nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()

	// Give the first attempt time to fail and enter the long backoff.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("backoff did not honor context cancellation")
	}
}

func TestBridgeRunnerBuildErrorIsNotRetried(t *testing.T) {
	t.Parallel()

	session := testSession(t, "http2")
	session.HAConnections = 1
	runner := NewBridgeRunner(session, newDiscardSlogLogger())
	runner.backoffBase = time.Millisecond

	var attempts int32
	runner.instanceFactory = func(_ Session, _ *slog.Logger, _ InstanceOptions) (bridgeInstance, error) {
		atomic.AddInt32(&attempts, 1)
		return nil, errors.New("bad config")
	}

	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("expected build error")
	}
	if strings.Contains(err.Error(), "exhausted") {
		t.Fatalf("build error should not be retried: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("expected exactly 1 attempt for build error, got %d", got)
	}
}

func TestBridgeRunnerBackoffDurationGrowsAndCaps(t *testing.T) {
	t.Parallel()

	runner := NewBridgeRunner(testSession(t, "http2"), newDiscardSlogLogger())
	runner.backoffBase = time.Second
	runner.backoffCap = 8 * time.Second

	cases := map[int]time.Duration{
		1: time.Second,
		2: 2 * time.Second,
		3: 4 * time.Second,
		4: 8 * time.Second,
		5: 8 * time.Second, // capped
	}
	for attempt, want := range cases {
		if got := runner.backoffDuration(attempt); got != want {
			t.Fatalf("attempt %d: got %v want %v", attempt, got, want)
		}
	}
}
