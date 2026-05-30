package cftunnel

import (
	"log/slog"

	tunnelruntime "github.com/jians1/cf-tunnel/internal/cftunnel/runtime"
)

func logTunnelSummary(logger *slog.Logger, requestedProtocol string, session tunnelruntime.Session) {
	if session.QuickTunnel {
		logQuickTunnelSummary(logger, requestedProtocol, session)
		return
	}
	logger.Info(
		"cloudflare tunnel ready",
		"tunnel_id", session.TunnelID,
		"protocol", requestedProtocol,
		"origin", session.Origin.URL,
	)
	logger.Debug(
		"cloudflare tunnel detail",
		"origin_protocol", session.Origin.Protocol,
		"origin_server_name", session.Origin.ServerName,
		"origin_insecure_skip_verify", session.Origin.InsecureSkipVerify,
		"origin_websocket_upgrade_mode", session.Origin.WebsocketUpgradeMode,
		"ha_connections", session.HAConnections,
	)
}

func logQuickTunnelSummary(logger *slog.Logger, requestedProtocol string, session tunnelruntime.Session) {
	logger.Info(
		"quick tunnel ready",
		"url", session.PublicURL,
		"protocol", requestedProtocol,
		"origin", session.Origin.URL,
	)
	logger.Debug(
		"quick tunnel detail",
		"hostname", session.Hostname,
		"tunnel_id", session.TunnelID,
		"origin_protocol", session.Origin.Protocol,
		"origin_server_name", session.Origin.ServerName,
		"origin_insecure_skip_verify", session.Origin.InsecureSkipVerify,
		"origin_websocket_upgrade_mode", session.Origin.WebsocketUpgradeMode,
		"ha_connections", session.HAConnections,
	)
}

func formatProtocol(protocol string) string {
	if protocol == "" {
		return "unknown"
	}
	return protocol
}
