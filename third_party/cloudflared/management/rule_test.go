package management_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/cloudflare/cloudflared/management"
)

func TestNewIngressRule(t *testing.T) {
	t.Parallel()

	logger := zerolog.Nop()
	service := management.New("management.argotunnel.com", false, "1.1.1.1:80", uuid.Nil, "", &logger, nil)

	rule := service.NewIngressRule()
	if rule.Hostname != "management.argotunnel.com" {
		t.Fatalf("unexpected hostname: %s", rule.Hostname)
	}
	if rule.Service.String() != "management" {
		t.Fatalf("unexpected service: %s", rule.Service.String())
	}
}
