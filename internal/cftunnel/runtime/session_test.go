package runtime

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jians1/cf-tunnel/internal/cftunnel/api"
	tunnelconfig "github.com/jians1/cf-tunnel/internal/cftunnel/config"
	"github.com/jians1/cf-tunnel/internal/cftunnel/credentials"
	"github.com/jians1/cf-tunnel/internal/cftunnel/origin"
)

func TestBuildSession(t *testing.T) {
	t.Parallel()

	cfg := tunnelconfig.RuntimeConfig{
		EdgeProtocol:       "quic",
		QuickService:       "https://api.trycloudflare.com",
		HAConnections:      1,
		QuickTunnelDefault: true,
		Origin: origin.Target{
			Raw:                  "127.0.0.1:8080",
			Protocol:             origin.ProtocolHTTP,
			URL:                  mustURL(t, "http://127.0.0.1:8080"),
			ServerName:           "example.com",
			InsecureSkipVerify:   true,
			WebsocketUpgradeMode: false,
		},
	}
	reservation := &api.QuickTunnelReservation{
		Credentials: credentials.Credentials{
			AccountTag:   "acct",
			TunnelSecret: []byte("secret"),
			TunnelID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		},
		Hostname: "demo.trycloudflare.com",
		URL:      "https://demo.trycloudflare.com",
	}

	session, err := BuildSession(cfg, reservation)
	if err != nil {
		t.Fatalf("build session: %v", err)
	}
	if session.TunnelID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected tunnel id: %s", session.TunnelID)
	}
	if session.Edge.Protocol != "quic" {
		t.Fatalf("unexpected edge protocol: %s", session.Edge.Protocol)
	}
	if session.Origin.URL != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected origin url: %s", session.Origin.URL)
	}
	if !session.Origin.InsecureSkipVerify {
		t.Fatal("expected insecure skip verify")
	}
}

func TestBuildTokenSession(t *testing.T) {
	t.Parallel()

	cfg := tunnelconfig.RuntimeConfig{
		EdgeProtocol:       "quic",
		HAConnections:      1,
		QuickTunnelDefault: false,
		Origin: origin.Target{
			Raw:      "http://127.0.0.1:8080",
			Protocol: origin.ProtocolHTTP,
			URL:      mustURL(t, "http://127.0.0.1:8080"),
		},
	}
	creds := credentials.Credentials{
		TunnelID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		AccountTag:   "account-tag",
		TunnelSecret: []byte("secret-value"),
	}

	session, err := BuildTokenSession(cfg, creds)
	if err != nil {
		t.Fatalf("build token session: %v", err)
	}
	if session.QuickTunnel {
		t.Fatal("expected formal tunnel session")
	}
	if session.Hostname != "" || session.PublicURL != "" {
		t.Fatalf("formal token session should not fake public URL: hostname=%q url=%q", session.Hostname, session.PublicURL)
	}
	if session.TunnelID != creds.TunnelID.String() {
		t.Fatalf("unexpected tunnel id %q", session.TunnelID)
	}
}

func TestBuildSessionRejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()

	baseCfg := tunnelconfig.RuntimeConfig{
		EdgeProtocol:       "quic",
		QuickService:       "https://api.trycloudflare.com",
		HAConnections:      1,
		QuickTunnelDefault: true,
		Origin: origin.Target{
			Raw:                  "127.0.0.1:8080",
			Protocol:             origin.ProtocolHTTP,
			URL:                  mustURL(t, "http://127.0.0.1:8080"),
			ServerName:           "example.com",
			InsecureSkipVerify:   true,
			WebsocketUpgradeMode: false,
		},
	}
	baseReservation := &api.QuickTunnelReservation{
		Credentials: credentials.Credentials{
			AccountTag:   "acct",
			TunnelSecret: []byte("secret"),
			TunnelID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		},
		Hostname: "demo.trycloudflare.com",
		URL:      "https://demo.trycloudflare.com",
	}

	tests := []struct {
		name    string
		mutate  func(*api.QuickTunnelReservation)
		wantErr string
	}{
		{
			name: "missing account tag",
			mutate: func(r *api.QuickTunnelReservation) {
				r.Credentials.AccountTag = ""
			},
			wantErr: "missing account tag",
		},
		{
			name: "missing tunnel secret",
			mutate: func(r *api.QuickTunnelReservation) {
				r.Credentials.TunnelSecret = nil
			},
			wantErr: "missing tunnel secret",
		},
		{
			name: "missing hostname",
			mutate: func(r *api.QuickTunnelReservation) {
				r.Hostname = ""
			},
			wantErr: "missing quick tunnel hostname or url",
		},
		{
			name: "missing url",
			mutate: func(r *api.QuickTunnelReservation) {
				r.URL = ""
			},
			wantErr: "missing quick tunnel hostname or url",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			reservation := *baseReservation
			tt.mutate(&reservation)

			_, err := BuildSession(baseCfg, &reservation)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
