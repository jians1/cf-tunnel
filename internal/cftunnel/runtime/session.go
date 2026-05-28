package runtime

import (
	"fmt"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/api"
	tunnelconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/config"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/origin"
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
}

func (s Session) ValidateRequiredQuickTunnelFields() error {
	if s.AccountTag == "" {
		return fmt.Errorf("missing account tag")
	}
	if len(s.Secret) == 0 {
		return fmt.Errorf("missing tunnel secret")
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
		},
		QuickTunnel:   cfg.QuickTunnelDefault,
		HAConnections: cfg.HAConnections,
	}
	if err := session.ValidateRequiredQuickTunnelFields(); err != nil {
		return Session{}, err
	}

	return session, nil
}
