package origin

import (
	"testing"

	appconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

func TestParseTargetHostPortHTTP(t *testing.T) {
	t.Parallel()

	target, err := ParseTarget("127.0.0.1:8080", appconfig.ProtocolHTTP, "", false)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	if target.URL.String() != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected url: %s", target.URL.String())
	}
	if target.WebsocketUpgradeMode {
		t.Fatal("unexpected websocket upgrade mode")
	}
}

func TestParseTargetURLWithWSOverride(t *testing.T) {
	t.Parallel()

	target, err := ParseTarget("https://127.0.0.1:8443/ws", appconfig.ProtocolWSS, "example.com", true)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	if target.URL.String() != "https://127.0.0.1:8443/ws" {
		t.Fatalf("unexpected url: %s", target.URL.String())
	}
	if !target.WebsocketUpgradeMode {
		t.Fatal("expected websocket upgrade mode")
	}
	if target.ServerName != "example.com" {
		t.Fatalf("unexpected server name: %s", target.ServerName)
	}
	if !target.InsecureSkipVerify {
		t.Fatal("expected insecure skip verify")
	}
}
