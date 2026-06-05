package cftunnel

import (
	"strings"
	"testing"

	"github.com/jians1/cf-tunnel/internal/config"
	apphealth "github.com/jians1/cf-tunnel/internal/health"
	tunnelruntime "github.com/jians1/cf-tunnel/internal/cftunnel/runtime"
)

func TestRunnerReadinessDefaultsToPending(t *testing.T) {
	t.Parallel()

	runner := NewRunner(config.CFTunnelConfig{
		EdgeProtocol: config.EdgeProtocolHTTP2,
		Target:       "http://127.0.0.1:8080",
	}, testLogger())

	status := runner.ReadyStatus()
	if status.Ready {
		t.Fatalf("expected runner to start not ready: %#v", status)
	}
	if !strings.Contains(status.Summary, "mode=single total=1 ready=0 failed=0 details=[cftunnel:pending]") {
		t.Fatalf("unexpected summary: %s", status.Summary)
	}
}

func TestRunnerReadinessMarksReadyWhenConnected(t *testing.T) {
	t.Parallel()

	runner := NewRunner(config.CFTunnelConfig{
		EdgeProtocol: config.EdgeProtocolHTTP2,
		Target:       "http://127.0.0.1:8080",
	}, testLogger())

	runner.markReadyForTest()

	status := runner.ReadyStatus()
	if !status.Ready {
		t.Fatalf("expected ready runner: %#v", status)
	}
	if !strings.Contains(status.Summary, "details=[cftunnel:ready]") {
		t.Fatalf("unexpected summary: %s", status.Summary)
	}
}

func TestRunnerStatusDefaultsToPendingSnapshot(t *testing.T) {
	t.Parallel()

	runner := NewRunner(config.CFTunnelConfig{
		EdgeProtocol: config.EdgeProtocolHTTP2,
		Target:       "http://127.0.0.1:8080",
	}, testLogger())

	status := runner.Status()
	if status.Mode != "single" || status.Ready {
		t.Fatalf("unexpected status payload: %#v", status)
	}
	if status.Tunnel == nil {
		t.Fatalf("expected tunnel payload: %#v", status)
	}
	if status.Tunnel.Status != "pending" || status.Tunnel.Protocol != config.EdgeProtocolHTTP2 || status.Tunnel.OriginURL != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected tunnel payload: %#v", status.Tunnel)
	}
}

func TestRunnerStatusIncludesQuickTunnelMetadata(t *testing.T) {
	t.Parallel()

	runner := NewRunner(config.CFTunnelConfig{
		EdgeProtocol: config.EdgeProtocolQUIC,
		Target:       "http://127.0.0.1:8080",
	}, testLogger())

	runner.setSessionForTest(tunnelruntime.Session{
		QuickTunnel: true,
		Hostname:    "demo.trycloudflare.com",
		PublicURL:   "https://demo.trycloudflare.com",
		Edge: tunnelruntime.EdgeSettings{
			Protocol: config.EdgeProtocolQUIC,
		},
		Origin: tunnelruntime.OriginSettings{
			URL: "http://127.0.0.1:8080",
		},
	})
	runner.markReadyForTest()

	status := runner.Status()
	if !status.Ready || status.Tunnel == nil {
		t.Fatalf("unexpected status payload: %#v", status)
	}
	if !status.Tunnel.QuickTunnel || status.Tunnel.QuickTunnelURL != "https://demo.trycloudflare.com" || status.Tunnel.Hostname != "demo.trycloudflare.com" {
		t.Fatalf("unexpected tunnel metadata: %#v", status.Tunnel)
	}
}

func TestBuildUserAgentUsesReleaseBinaryName(t *testing.T) {
	t.Parallel()

	if got := buildUserAgent(); got != "cf-tunnel/dev" {
		t.Fatalf("unexpected user agent: %s", got)
	}
}

var _ apphealth.ReadyProvider = (*Runner)(nil).ReadyStatus
