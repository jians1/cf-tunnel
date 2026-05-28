package cftunnel

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/api"
	tunnelconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/config"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

func TestPrepareQuickTunnelSessionWithMockReservation(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	prepared, err := prepareQuickTunnelSessionWith(context.Background(), config.CFTunnelConfig{
		EdgeProtocol:   config.EdgeProtocolHTTP2,
		Target:         "127.0.0.1:8080",
		OriginProtocol: config.ProtocolHTTP,
	}, logger, mockQuickTunnelReservationFunc())
	if err != nil {
		t.Fatalf("prepare session: %v", err)
	}
	if prepared.reservation == nil {
		t.Fatal("expected reservation")
	}
	if prepared.session.Hostname != "demo.trycloudflare.com" {
		t.Fatalf("unexpected hostname: %s", prepared.session.Hostname)
	}
}

func TestPrepareQuickTunnelSessionWithCarriesQuickServiceOptions(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := prepareQuickTunnelSessionWith(context.Background(), config.CFTunnelConfig{
		EdgeProtocol:   config.EdgeProtocolHTTP2,
		Target:         "127.0.0.1:8080",
		OriginProtocol: config.ProtocolHTTP,
	}, logger, func(_ context.Context, runtimeConfig tunnelconfig.RuntimeConfig) (*api.QuickTunnelReservation, error) {
		if runtimeConfig.QuickServiceTimeout != 15*time.Second {
			t.Fatalf("unexpected quick service timeout: %s", runtimeConfig.QuickServiceTimeout)
		}
		if len(runtimeConfig.RetryBackoffs) != 2 {
			t.Fatalf("unexpected retry backoff size: %d", len(runtimeConfig.RetryBackoffs))
		}
		if runtimeConfig.RetryBackoffs[0] != 500*time.Millisecond || runtimeConfig.RetryBackoffs[1] != 1500*time.Millisecond {
			t.Fatalf("unexpected retry backoffs: %#v", runtimeConfig.RetryBackoffs)
		}
		return mockQuickTunnelReservationFunc()(context.Background(), runtimeConfig)
	})
	if err != nil {
		t.Fatalf("prepare session: %v", err)
	}
}
