package runtime

import (
	"fmt"

	"github.com/jians1/cf-tunnel/internal/cftunnel/api"
	tunnelconfig "github.com/jians1/cf-tunnel/internal/cftunnel/config"
	"github.com/jians1/cf-tunnel/internal/cftunnel/credentials"
	"github.com/jians1/cf-tunnel/internal/cftunnel/origin"
	appconfig "github.com/jians1/cf-tunnel/internal/config"
)

type Session struct {
	TunnelID      string
	AccountTag    string
	Secret        []byte
	Hostname      string
	PublicURL     string
	Edge          EdgeSettings
	Origin        OriginSettings
	QuickTunnel   bool
	HAConnections int
}

type EdgeSettings struct {
	Protocol string
}

type OriginSettings struct {
	RawTarget            string
	URL                  string
	Protocol             origin.Protocol
	ServerName           string
	InsecureSkipVerify   bool
	WebsocketUpgradeMode bool
	Routes               []appconfig.RouteRule
}

func (s Session) ValidateRequiredCredentialFields() error {
	if s.TunnelID == "" {
		return fmt.Errorf("missing tunnel id")
	}
	if s.AccountTag == "" {
		return fmt.Errorf("missing account tag")
	}
	if len(s.Secret) == 0 {
		return fmt.Errorf("missing tunnel secret")
	}
	return nil
}

func (s Session) ValidateRequiredQuickTunnelFields() error {
	if err := s.ValidateRequiredCredentialFields(); err != nil {
		return err
	}
	if s.Hostname == "" {
		return fmt.Errorf("missing quick tunnel hostname")
	}
	return nil
}

func BuildSession(cfg tunnelconfig.RuntimeConfig, reservation *api.QuickTunnelReservation) (Session, error) {
	if reservation == nil {
		return Session{}, fmt.Errorf("nil quick tunnel reservation")
	}
	if reservation.Credentials.TunnelID.String() == "" {
		return Session{}, fmt.Errorf("missing tunnel id")
	}
	if reservation.Hostname == "" || reservation.URL == "" {
		return Session{}, fmt.Errorf("missing quick tunnel hostname or url")
	}
	session := Session{
		TunnelID:   reservation.Credentials.TunnelID.String(),
		AccountTag: reservation.Credentials.AccountTag,
		Secret:     append([]byte(nil), reservation.Credentials.TunnelSecret...),
		Hostname:   reservation.Hostname,
		PublicURL:  reservation.URL,
		Edge: EdgeSettings{
			Protocol: cfg.EdgeProtocol,
		},
		Origin: OriginSettings{
			RawTarget:            cfg.Origin.Raw,
			URL:                  cfg.Origin.URL.String(),
			Protocol:             cfg.Origin.Protocol,
			ServerName:           cfg.Origin.ServerName,
			InsecureSkipVerify:   cfg.Origin.InsecureSkipVerify,
			WebsocketUpgradeMode: cfg.Origin.WebsocketUpgradeMode,
			Routes:               append([]appconfig.RouteRule(nil), cfg.Routes...),
		},
		QuickTunnel:   cfg.QuickTunnelDefault,
		HAConnections: cfg.HAConnections,
	}
	if err := session.ValidateRequiredQuickTunnelFields(); err != nil {
		return Session{}, err
	}

	return session, nil
}

func BuildTokenSession(cfg tunnelconfig.RuntimeConfig, creds credentials.Credentials) (Session, error) {
	if creds.TunnelID.String() == "" {
		return Session{}, fmt.Errorf("missing tunnel id")
	}
	session := Session{
		TunnelID:   creds.TunnelID.String(),
		AccountTag: creds.AccountTag,
		Secret:     append([]byte(nil), creds.TunnelSecret...),
		Edge: EdgeSettings{
			Protocol: cfg.EdgeProtocol,
		},
		Origin: OriginSettings{
			RawTarget:            cfg.Origin.Raw,
			URL:                  cfg.Origin.URL.String(),
			Protocol:             cfg.Origin.Protocol,
			ServerName:           cfg.Origin.ServerName,
			InsecureSkipVerify:   cfg.Origin.InsecureSkipVerify,
			WebsocketUpgradeMode: cfg.Origin.WebsocketUpgradeMode,
			Routes:               append([]appconfig.RouteRule(nil), cfg.Routes...),
		},
		QuickTunnel:   false,
		HAConnections: cfg.HAConnections,
	}
	if err := session.ValidateRequiredCredentialFields(); err != nil {
		return Session{}, err
	}
	return session, nil
}
