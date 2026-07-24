package origin

import "testing"

func TestParseTargetHTTPURL(t *testing.T) {
	t.Parallel()

	target, err := ParseTarget("http://127.0.0.1:8080", "", false, false)
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	if target.URL.String() != "http://127.0.0.1:8080" {
		t.Fatalf("unexpected url: %s", target.URL.String())
	}
	if target.WebsocketUpgradeMode {
		t.Fatal("unexpected websocket upgrade mode")
	}
	if target.PassHostHeader {
		t.Fatal("unexpected pass host header")
	}
}

func TestParseTargetURLWithWSOverride(t *testing.T) {
	t.Parallel()

	target, err := ParseTarget("wss://127.0.0.1:8443/ws", "example.com", true, true)
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
	if !target.PassHostHeader {
		t.Fatal("expected pass host header")
	}
}
