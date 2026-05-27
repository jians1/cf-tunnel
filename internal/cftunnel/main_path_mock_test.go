package cftunnel

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	tunnelconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/config"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/runtime"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

func TestMainPathWithMockQuickTunnelReservation(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.CFTunnelConfig{
		Enabled:        true,
		EdgeProtocol:   config.EdgeProtocolHTTP2,
		Target:         "127.0.0.1:8080",
		OriginProtocol: config.ProtocolHTTP,
	}

	prepared, err := prepareQuickTunnelSessionWith(context.Background(), cfg, logger, mockQuickTunnelReservationFunc())
	if err != nil {
		t.Fatalf("prepare session: %v", err)
	}

	bridge := runtime.NewBridgeRunner(prepared.session, logger)
	bridge.SetHTTP2Options(runtime.HTTP2ServerOptions{})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := bridge.Run(ctx); err != nil &&
		err != context.DeadlineExceeded &&
		err != context.Canceled &&
		!strings.Contains(err.Error(), "connection with edge closed") {
		t.Fatalf("bridge run: %v", err)
	}
}

func TestBuildHTTP2ServerOptionsDefaultsToEdgeDiscovery(t *testing.T) {
	t.Parallel()

	opts := buildHTTP2ServerOptions(tunnelRuntimeConfig(t), slog.New(slog.NewTextHandler(io.Discard, nil)))

	if opts.DialAddress != "" {
		t.Fatalf("unexpected http2 dial address: %s", opts.DialAddress)
	}
	if opts.EdgeAddressProvider == nil {
		t.Fatal("expected default http2 edge address provider")
	}
	if opts.DialTimeout == 0 {
		t.Fatal("expected http2 dial timeout")
	}
}

func TestBuildQUICRuntimeOptionsDefaultsToEdgeDiscovery(t *testing.T) {
	t.Parallel()

	opts := buildQUICRuntimeOptions(config.CFTunnelConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if opts.DialAddress != "" {
		t.Fatalf("unexpected quic dial address: %s", opts.DialAddress)
	}
	if opts.EdgeAddressProvider == nil {
		t.Fatal("expected default quic edge address provider")
	}
	if opts.DialTimeout == 0 {
		t.Fatal("expected quic dial timeout")
	}
}

func tunnelRuntimeConfig(t *testing.T) tunnelconfig.RuntimeConfig {
	t.Helper()
	cfg, err := tunnelconfig.Normalize(config.CFTunnelConfig{
		Enabled:        true,
		EdgeProtocol:   config.EdgeProtocolHTTP2,
		Target:         "127.0.0.1:8080",
		OriginProtocol: config.ProtocolHTTP,
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	return cfg
}
