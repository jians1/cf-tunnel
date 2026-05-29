package cftunnel

import (
	"context"
	"encoding/base64"
	"errors"
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
		EdgeProtocol: config.EdgeProtocolHTTP2,
		Target:       "http://127.0.0.1:8080",
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

func TestPrepareSessionWithTokenSkipsQuickTunnelReservation(t *testing.T) {
	t.Parallel()

	token := testTunnelToken(t)
	called := false
	prepared, err := prepareTunnelSessionWith(context.Background(), config.CFTunnelConfig{
		TunnelToken:  token,
		EdgeProtocol: config.EdgeProtocolQUIC,
		Target:       "http://127.0.0.1:8080",
	}, slog.Default(), func(context.Context, tunnelconfig.RuntimeConfig) (*api.QuickTunnelReservation, error) {
		called = true
		return nil, errors.New("reservation should not be called")
	})
	if err != nil {
		t.Fatalf("prepare token session: %v", err)
	}
	if called {
		t.Fatal("quick tunnel reservation was called in token mode")
	}
	if prepared.session.QuickTunnel {
		t.Fatal("expected formal tunnel session")
	}
	if prepared.reservation != nil {
		t.Fatal("expected no quick tunnel reservation")
	}
}

func testTunnelToken(t *testing.T) string {
	t.Helper()

	secret := base64.StdEncoding.EncodeToString([]byte("secret-value"))
	raw := `{"a":"account-tag","t":"11111111-1111-1111-1111-111111111111","s":"` + secret + `"}`
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func TestPrepareQuickTunnelSessionWithCarriesQuickServiceOptions(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := prepareQuickTunnelSessionWith(context.Background(), config.CFTunnelConfig{
		EdgeProtocol: config.EdgeProtocolHTTP2,
		Target:       "http://127.0.0.1:8080",
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
