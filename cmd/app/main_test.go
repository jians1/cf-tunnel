package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jians1/cf-tunnel/internal/config"
	"github.com/jians1/cf-tunnel/internal/health"
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

func TestBuildRunnersWiresHealthStatusProviderForSingleTunnel(t *testing.T) {
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

	hr, ok := runners[0].(*health.Runner)
	if !ok {
		t.Fatalf("first runner should be health runner, got %T", runners[0])
	}

	status := hr.Status()
	if status.Mode != "single" || status.Tunnel == nil {
		t.Fatalf("unexpected status payload: %#v", status)
	}
	if status.Tunnel.Protocol != config.EdgeProtocolHTTP2 || status.Tunnel.OriginURL != "http://127.0.0.1:8081" {
		t.Fatalf("unexpected tunnel payload: %#v", status.Tunnel)
	}
}

func TestBuildRunnersWiresHealthStatusProviderForMultiTunnel(t *testing.T) {
	t.Parallel()

	cfg := config.AppConfig{
		HealthListen: ":9090",
		Tunnels: []config.NamedTunnelConfig{
			{
				Name: "alpha",
				CFTunnel: config.CFTunnelConfig{
					EdgeProtocol: config.EdgeProtocolQUIC,
					Target:       "http://127.0.0.1:8081",
				},
			},
			{
				Name: "beta",
				CFTunnel: config.CFTunnelConfig{
					EdgeProtocol: config.EdgeProtocolHTTP2,
					Target:       "http://127.0.0.1:8082",
				},
			},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	runners, err := buildRunners(cfg, logger)
	if err != nil {
		t.Fatalf("build runners: %v", err)
	}

	hr, ok := runners[0].(*health.Runner)
	if !ok {
		t.Fatalf("first runner should be health runner, got %T", runners[0])
	}

	status := hr.Status()
	if status.Mode != "multi" || len(status.Tunnels) != 2 {
		t.Fatalf("unexpected status payload: %#v", status)
	}

	// Keep one JSON round-trip to ensure the health transport payload shape stays serializable.
	b, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	if !strings.Contains(string(b), `"mode":"multi"`) {
		t.Fatalf("unexpected status json: %s", string(b))
	}
}
