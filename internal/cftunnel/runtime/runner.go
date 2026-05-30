package runtime

import (
	"context"
	"fmt"
	"log/slog"
)

type BridgeRunner struct {
	session      Session
	logger       *slog.Logger
	http2Options HTTP2ServerOptions
	quicOptions  QUICRuntimeOptions
}

func NewBridgeRunner(session Session, logger *slog.Logger) *BridgeRunner {
	if logger == nil {
		logger = slog.Default()
	}

	return &BridgeRunner{
		session: session,
		logger:  logger.With("component", "cftunnel-runtime"),
	}
}

func (r *BridgeRunner) SetHTTP2Options(opts HTTP2ServerOptions) {
	r.http2Options = opts
}

func (r *BridgeRunner) SetQUICOptions(opts QUICRuntimeOptions) {
	r.quicOptions = opts
}

func (r *BridgeRunner) Run(ctx context.Context) error {
	r.logger.Debug(
		"cftunnel runtime bridge prepared",
		"tunnel_id", r.session.TunnelID,
		"hostname", r.session.Hostname,
		"public_url", r.session.PublicURL,
		"edge_protocol", r.session.Edge.Protocol,
		"origin_url", r.session.Origin.URL,
		"origin_protocol", r.session.Origin.Protocol,
		"origin_server_name", r.session.Origin.ServerName,
		"origin_insecure_skip_verify", r.session.Origin.InsecureSkipVerify,
		"origin_websocket_upgrade_mode", r.session.Origin.WebsocketUpgradeMode,
		"ha_connections", r.session.HAConnections,
	)

	instance, err := NewInstanceWithRuntimeOptions(r.session, r.logger, InstanceOptions{
		HTTP2: r.http2Options,
		QUIC:  r.quicOptions,
	})
	if err != nil {
		return fmt.Errorf("build runtime instance: %w", err)
	}
	if instance.HTTP2Server != nil {
		r.logger.Debug("http2 runtime server composed")
		if r.http2Options.LocalEdgeDriver {
			driver, err := NewHTTP2LocalEdgeDriver(instance.HTTP2Server)
			if err != nil {
				return err
			}
			return driver.Run(ctx)
		}
		return instance.Run(ctx)
	}
	return instance.Run(ctx)
}
