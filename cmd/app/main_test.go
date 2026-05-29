package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/health"
)

func TestBuildRunnersWiresHealthReadySummaryForMultiTunnel(t *testing.T) {
	t.Parallel()

	cfg := config.AppConfig{
		HealthListen: ":9090",
		Tunnels: []config.NamedTunnelConfig{
			{
				Name: "alpha",
				CFTunnel: config.CFTunnelConfig{
					EdgeProtocol: config.EdgeProtocolHTTP2,
					Target:       "http://127.0.0.1:8081",
				},
			},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	runners, err := buildRunners(cfg, logger)
	if err != nil {
		t.Fatalf("build runners: %v", err)
	}
	if len(runners) != 2 {
		t.Fatalf("unexpected runner count: %d", len(runners))
	}

	hr, ok := runners[0].(*health.Runner)
	if !ok {
		t.Fatalf("first runner should be health runner, got %T", runners[0])
	}
	status := hr.ReadyStatus()
	if status.Ready {
		t.Fatalf("multi tunnel should not be ready before registration: %#v", status)
	}
	if !strings.Contains(status.Summary, "mode=multi total=1 ready=0") {
		t.Fatalf("unexpected ready summary: %s", status.Summary)
	}
}

func TestBuildRunnersWiresHealthReadySummaryForSingleTunnel(t *testing.T) {
	t.Parallel()

	cfg := config.AppConfig{
		HealthListen: ":9090",
		CFTunnel: config.CFTunnelConfig{
			EdgeProtocol: config.EdgeProtocolHTTP2,
			Target:       "http://127.0.0.1:8081",
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	runners, err := buildRunners(cfg, logger)
	if err != nil {
		t.Fatalf("build runners: %v", err)
	}
	if len(runners) != 2 {
		t.Fatalf("unexpected runner count: %d", len(runners))
	}
	hr, ok := runners[0].(*health.Runner)
	if !ok {
		t.Fatalf("first runner should be health runner, got %T", runners[0])
	}
	status := hr.ReadyStatus()
	if status.Ready {
		t.Fatalf("single tunnel should not be ready before registration: %#v", status)
	}
	if !strings.Contains(status.Summary, "mode=single total=1 ready=0") {
		t.Fatalf("unexpected ready summary: %s", status.Summary)
	}
}
