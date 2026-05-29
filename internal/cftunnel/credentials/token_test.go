package credentials

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParseTunnelToken(t *testing.T) {
	t.Parallel()

	tunnelID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	secret := base64.StdEncoding.EncodeToString([]byte("secret-value"))
	raw := `{"a":"account-tag","t":"` + tunnelID.String() + `","s":"` + secret + `","e":"edge.example.com"}`
	token := base64.StdEncoding.EncodeToString([]byte(raw))

	parsed, err := ParseTunnelToken(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if parsed.AccountTag != "account-tag" {
		t.Fatalf("unexpected account tag %q", parsed.AccountTag)
	}
	if parsed.TunnelID != tunnelID {
		t.Fatalf("unexpected tunnel id %s", parsed.TunnelID)
	}
	if string(parsed.TunnelSecret) != "secret-value" {
		t.Fatalf("unexpected secret %q", string(parsed.TunnelSecret))
	}
	if parsed.Endpoint != "edge.example.com" {
		t.Fatalf("unexpected endpoint %q", parsed.Endpoint)
	}
}

func TestParseTunnelTokenRejectsMalformedBase64(t *testing.T) {
	t.Parallel()

	_, err := ParseTunnelToken("not base64")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "decode tunnel token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTunnelTokenRejectsEmptyToken(t *testing.T) {
	t.Parallel()

	_, err := ParseTunnelToken(" ")
	if err == nil {
		t.Fatal("expected missing token error")
	}
	if !strings.Contains(err.Error(), "missing tunnel token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTunnelTokenRejectsInvalidTunnelID(t *testing.T) {
	t.Parallel()

	secret := base64.StdEncoding.EncodeToString([]byte("secret-value"))
	raw := `{"a":"account-tag","t":"not-a-uuid","s":"` + secret + `"}`
	token := base64.StdEncoding.EncodeToString([]byte(raw))
	_, err := ParseTunnelToken(token)
	if err == nil {
		t.Fatal("expected invalid tunnel id error")
	}
	if !strings.Contains(err.Error(), "unmarshal tunnel token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTunnelTokenRejectsMissingSecret(t *testing.T) {
	t.Parallel()

	raw := `{"a":"account-tag","t":"11111111-1111-1111-1111-111111111111"}`
	token := base64.StdEncoding.EncodeToString([]byte(raw))
	_, err := ParseTunnelToken(token)
	if err == nil {
		t.Fatal("expected missing secret error")
	}
	if !strings.Contains(err.Error(), "missing tunnel token secret") {
		t.Fatalf("unexpected error: %v", err)
	}
}
