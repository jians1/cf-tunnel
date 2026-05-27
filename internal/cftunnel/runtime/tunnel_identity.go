package runtime

import (
	"github.com/cloudflare/cloudflared/tunnelrpc/pogs"
	"github.com/google/uuid"
)

type RuntimeCredentials struct {
	AccountTag   string
	TunnelSecret []byte
	TunnelID     uuid.UUID
	Endpoint     string
}

func (c *RuntimeCredentials) Auth() pogs.TunnelAuth {
	return pogs.TunnelAuth{
		AccountTag:   c.AccountTag,
		TunnelSecret: c.TunnelSecret,
	}
}

type RuntimeTunnelProperties struct {
	Credentials    RuntimeCredentials
	QuickTunnelURL string
}
