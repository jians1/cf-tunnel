package cftunnel

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

func TestPrepareQuickTunnelSessionWithMockReservation(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	prepared, err := prepareQuickTunnelSessionWith(context.Background(), config.CFTunnelConfig{
		Enabled:        true,
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
