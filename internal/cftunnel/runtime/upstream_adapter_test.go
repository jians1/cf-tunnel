package runtime

import (
	"strings"
	"testing"
)

func TestUpstreamAdapterBindQUIC(t *testing.T) {
	t.Parallel()

	adapter := NewUpstreamAdapter()
	binding, err := adapter.Bind(testSession(t, "quic"))
	if err != nil {
		t.Fatalf("bind upstream: %v", err)
	}
	if binding.Credentials.AccountTag != "acct" {
		t.Fatalf("unexpected account tag: %s", binding.Credentials.AccountTag)
	}
	if binding.TunnelProperties.QuickTunnelURL != "demo.trycloudflare.com" {
		t.Fatalf("unexpected quick tunnel url: %s", binding.TunnelProperties.QuickTunnelURL)
	}
	if got := binding.ProtocolSelector.Current().String(); got != "quic" {
		t.Fatalf("unexpected protocol: %s", got)
	}
}

func TestUpstreamAdapterBindHTTP2(t *testing.T) {
	t.Parallel()

	adapter := NewUpstreamAdapter()
	binding, err := adapter.Bind(testSession(t, "http2"))
	if err != nil {
		t.Fatalf("bind upstream: %v", err)
	}
	if got := binding.ProtocolSelector.Current().String(); got != "http2" {
		t.Fatalf("unexpected protocol: %s", got)
	}
}

func TestUpstreamAdapterBindsFormalTunnelWithoutHostname(t *testing.T) {
	t.Parallel()

	session := Session{
		TunnelID:   "11111111-1111-1111-1111-111111111111",
		AccountTag: "account-tag",
		Secret:     []byte("secret-value"),
		Edge:       EdgeSettings{Protocol: "quic"},
		Origin:     OriginSettings{RawTarget: "http://127.0.0.1:8080"},
	}

	binding, err := NewUpstreamAdapter().Bind(session)
	if err != nil {
		t.Fatalf("bind upstream: %v", err)
	}
	if binding.TunnelProperties.QuickTunnelURL != "" {
		t.Fatalf("expected no quick tunnel url, got %q", binding.TunnelProperties.QuickTunnelURL)
	}
}

func TestUpstreamAdapterRejectsInvalidTunnelID(t *testing.T) {
	t.Parallel()

	session := testSession(t, "quic")
	session.TunnelID = "not-a-uuid"

	adapter := NewUpstreamAdapter()
	if _, err := adapter.Bind(session); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpstreamAdapterRejectsMissingRequiredSessionFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Session)
		wantErr string
	}{
		{
			name: "account tag",
			mutate: func(session *Session) {
				session.AccountTag = ""
			},
			wantErr: "missing account tag",
		},
		{
			name: "tunnel secret",
			mutate: func(session *Session) {
				session.Secret = nil
			},
			wantErr: "missing tunnel secret",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			session := testSession(t, "quic")
			tt.mutate(&session)

			adapter := NewUpstreamAdapter()
			_, err := adapter.Bind(session)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q error, got: %v", tt.wantErr, err)
			}
		})
	}
}
