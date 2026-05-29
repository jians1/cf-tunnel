package cftunnel

import (
	"strings"
	"testing"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
	apphealth "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/health"
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

var _ apphealth.ReadyProvider = (*Runner)(nil).ReadyStatus
