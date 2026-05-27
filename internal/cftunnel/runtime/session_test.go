package runtime

import (
	"testing"

	"github.com/google/uuid"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/api"
	tunnelconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/config"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/credentials"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/origin"
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
