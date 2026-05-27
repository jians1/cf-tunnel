package runtime

import (
	"github.com/google/uuid"
)

type RuntimeCredentials struct {
	AccountTag   string
	TunnelSecret []byte
	TunnelID     uuid.UUID
	Endpoint     string
}

func (c *RuntimeCredentials) Auth() runtimeTunnelAuth {
	return runtimeTunnelAuth{
		AccountTag:   c.AccountTag,
		TunnelSecret: c.TunnelSecret,
	}
}

type RuntimeTunnelProperties struct {
	Credentials    RuntimeCredentials
	QuickTunnelURL string
}
