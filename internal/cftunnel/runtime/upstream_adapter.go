package runtime

import (
	"fmt"
	"time"

	cfdconnection "github.com/cloudflare/cloudflared/connection"
	cfdedgediscovery "github.com/cloudflare/cloudflared/edgediscovery"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type UpstreamAdapter struct {
	logger zerolog.Logger
}

type UpstreamBinding struct {
	Credentials      cfdconnection.Credentials
	TunnelProperties *cfdconnection.TunnelProperties
	ProtocolSelector cfdconnection.ProtocolSelector
}

func NewUpstreamAdapter() *UpstreamAdapter {
	return &UpstreamAdapter{
		logger: zerolog.Nop(),
	}
}

func (a *UpstreamAdapter) Bind(session Session) (*UpstreamBinding, error) {
	tunnelID, err := uuid.Parse(session.TunnelID)
	if err != nil {
		return nil, fmt.Errorf("parse tunnel id: %w", err)
	}
	if session.AccountTag == "" {
		return nil, fmt.Errorf("missing account tag")
	}
	if len(session.Secret) == 0 {
		return nil, fmt.Errorf("missing tunnel secret")
	}
	if session.Hostname == "" {
		return nil, fmt.Errorf("missing quick tunnel hostname")
	}

	credentials := cfdconnection.Credentials{
		AccountTag:   session.AccountTag,
		TunnelSecret: append([]byte(nil), session.Secret...),
		TunnelID:     tunnelID,
	}
	tunnelProperties := &cfdconnection.TunnelProperties{
		Credentials:    credentials,
		QuickTunnelUrl: session.Hostname,
	}

	protocolSelector, err := cfdconnection.NewProtocolSelector(
		session.Edge.Protocol,
		session.AccountTag,
		false,
		staticProtocolPercentages(),
		time.Hour,
		&a.logger,
	)
	if err != nil {
		return nil, fmt.Errorf("build upstream protocol selector: %w", err)
	}

	return &UpstreamBinding{
		Credentials:      credentials,
		TunnelProperties: tunnelProperties,
		ProtocolSelector: protocolSelector,
	}, nil
}

func staticProtocolPercentages() cfdedgediscovery.PercentageFetcher {
	return func() (cfdedgediscovery.ProtocolPercents, error) {
		return cfdedgediscovery.ProtocolPercents{
			{Protocol: "quic", Percentage: 100},
			{Protocol: "http2", Percentage: 0},
		}, nil
	}
}
