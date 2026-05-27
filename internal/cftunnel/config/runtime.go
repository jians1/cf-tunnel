package config

import (
	"fmt"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/origin"
	appconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

type RuntimeConfig struct {
	EdgeProtocol       string
	QuickService       string
	HAConnections      int
	Origin             origin.Target
	QuickTunnelDefault bool
}

func Normalize(cfg appconfig.CFTunnelConfig) (RuntimeConfig, error) {
	target, err := origin.ParseTarget(
		cfg.Target,
		cfg.OriginProtocol,
		cfg.OriginServerName,
		cfg.InsecureSkipVerify,
	)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("parse origin target: %w", err)
	}

	haConnections := cfg.HAConnections
	if haConnections == 0 {
		haConnections = 1
	}

	return RuntimeConfig{
		EdgeProtocol:       cfg.EdgeProtocol,
		QuickService:       cfg.QuickService,
		HAConnections:      haConnections,
		Origin:             target,
		QuickTunnelDefault: true,
	}, nil
}
