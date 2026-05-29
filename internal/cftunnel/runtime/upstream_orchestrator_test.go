package runtime

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestNewUpstreamOrchestrator(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareRuntime(testSession(t, "quic"))
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}
	originProxy := NewUpstreamOriginProxy(prepared.OriginProxy)

	orchestrator, err := NewUpstreamOrchestrator(originProxy, prepared.Session)
	if err != nil {
		t.Fatalf("new upstream orchestrator: %v", err)
	}

	cfg, err := orchestrator.GetConfigJSON()
	if err != nil {
		t.Fatalf("get config json: %v", err)
	}
	if !strings.Contains(string(cfg), `"protocol":"quic"`) {
		t.Fatalf("unexpected config json: %s", string(cfg))
	}
	var parsed upstreamOrchestratorConfig
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		t.Fatalf("unmarshal config json: %v", err)
	}
	if !parsed.QuickTunnel {
		t.Fatalf("expected quick tunnel config, got: %s", string(cfg))
	}

	gotProxy, err := orchestrator.GetOriginProxy()
	if err != nil {
		t.Fatalf("get origin proxy: %v", err)
	}
	if gotProxy == nil {
		t.Fatal("expected origin proxy")
	}

	resp := orchestrator.UpdateConfig(7, nil)
	if resp.LastAppliedVersion != 7 {
		t.Fatalf("unexpected applied version: %d", resp.LastAppliedVersion)
	}
}

func TestNewUpstreamOrchestratorRejectsNilProxy(t *testing.T) {
	t.Parallel()

	if _, err := NewUpstreamOrchestrator(nil, testSession(t, "http2")); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewUpstreamOrchestratorMarksFormalTunnelConfig(t *testing.T) {
	t.Parallel()

	session := testSession(t, "quic")
	session.QuickTunnel = false
	session.Hostname = ""
	session.PublicURL = ""
	prepared, err := PrepareRuntime(session)
	if err != nil {
		t.Fatalf("prepare runtime: %v", err)
	}

	orchestrator, err := NewUpstreamOrchestrator(NewUpstreamOriginProxy(prepared.OriginProxy), prepared.Session)
	if err != nil {
		t.Fatalf("new upstream orchestrator: %v", err)
	}

	cfg, err := orchestrator.GetConfigJSON()
	if err != nil {
		t.Fatalf("get config json: %v", err)
	}
	var parsed upstreamOrchestratorConfig
	if err := json.Unmarshal(cfg, &parsed); err != nil {
		t.Fatalf("unmarshal config json: %v", err)
	}
	if parsed.QuickTunnel {
		t.Fatalf("expected formal tunnel config, got: %s", string(cfg))
	}
}

var _ http.Handler = http.Handler(nil)
