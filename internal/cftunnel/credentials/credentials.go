package credentials

import "github.com/google/uuid"

type Credentials struct {
	AccountTag   string
	TunnelSecret []byte
	TunnelID     uuid.UUID
	Endpoint     string
}
