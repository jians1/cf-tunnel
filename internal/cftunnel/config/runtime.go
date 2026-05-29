package config

import (
	"fmt"
	"time"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/origin"
	appconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

var defaultQuickServiceRetryBackoffs = []time.Duration{
	500 * time.Millisecond,
	1500 * time.Millisecond,
}

type RuntimeConfig struct {
	EdgeProtocol        string
	QuickService        string
	QuickServiceTimeout time.Duration
	RetryBackoffs       []time.Duration
	HAConnections       int
	Origin              origin.Target
	Routes              []appconfig.RouteRule
	QuickTunnelDefault  bool
}

func Normalize(cfg appconfig.CFTunnelConfig) (RuntimeConfig, error) {
	target, err := origin.ParseTarget(
		cfg.Target,
		cfg.OriginServerName,
		cfg.InsecureSkipVerify,
	)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("parse origin target: %w", err)
	}

	return RuntimeConfig{
		EdgeProtocol:        cfg.EdgeProtocol,
		QuickService:        cfg.QuickService,
		QuickServiceTimeout: 15 * time.Second,
		RetryBackoffs:       append([]time.Duration(nil), defaultQuickServiceRetryBackoffs...),
		HAConnections:       1,
		Origin:              target,
		Routes:              append([]appconfig.RouteRule(nil), cfg.Routes...),
		QuickTunnelDefault:  true,
	}, nil
}
