package runtime

import (
	"fmt"

	"github.com/google/uuid"
)

type UpstreamAdapter struct {
}

type UpstreamBinding struct {
	Credentials      RuntimeCredentials
	TunnelProperties *RuntimeTunnelProperties
	ProtocolSelector ProtocolSelector
}

func NewUpstreamAdapter() *UpstreamAdapter { return &UpstreamAdapter{} }

func (a *UpstreamAdapter) Bind(session Session) (*UpstreamBinding, error) {
	tunnelID, err := uuid.Parse(session.TunnelID)
	if err != nil {
		return nil, fmt.Errorf("parse tunnel id: %w", err)
	}
	if err := session.ValidateRequiredCredentialFields(); err != nil {
		return nil, err
	}

	credentials := RuntimeCredentials{
		AccountTag:   session.AccountTag,
		TunnelSecret: append([]byte(nil), session.Secret...),
		TunnelID:     tunnelID,
	}
	tunnelProperties := &RuntimeTunnelProperties{
		Credentials:    credentials,
		QuickTunnelURL: session.Hostname,
	}

	protocolSelector, err := NewStaticProtocolSelector(session.Edge.Protocol)
	if err != nil {
		return nil, fmt.Errorf("build upstream protocol selector: %w", err)
	}

	return &UpstreamBinding{
		Credentials:      credentials,
		TunnelProperties: tunnelProperties,
		ProtocolSelector: protocolSelector,
	}, nil
}
