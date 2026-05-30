package cftunnel

import (
	"log/slog"
	"strings"

	tunnelruntime "github.com/jians1/cf-tunnel/internal/cftunnel/runtime"
)

func logTunnelSummary(logger *slog.Logger, requestedProtocol string, session tunnelruntime.Session) {
	if session.QuickTunnel {
		logQuickTunnelSummary(logger, requestedProtocol, session)
		return
	}
	logger.Info(
		"formal cloudflare tunnel ready",
		"mode", "main-run",
		"requested_protocol", requestedProtocol,
		"tunnel_id", session.TunnelID,
		"origin_url", session.Origin.URL,
		"origin_protocol", session.Origin.Protocol,
		"origin_server_name", session.Origin.ServerName,
		"origin_insecure_skip_verify", session.Origin.InsecureSkipVerify,
		"origin_websocket_upgrade_mode", session.Origin.WebsocketUpgradeMode,
		"ha_connections", session.HAConnections,
	)
}

func logQuickTunnelSummary(logger *slog.Logger, requestedProtocol string, session tunnelruntime.Session) {
	logger.Info(
		"cftunnel startup summary",
		"mode", "main-run",
		"requested_protocol", requestedProtocol,
		"quick_tunnel_hostname", session.Hostname,
		"quick_tunnel_url", session.PublicURL,
		"origin_url", session.Origin.URL,
		"origin_protocol", session.Origin.Protocol,
		"origin_server_name", session.Origin.ServerName,
		"origin_insecure_skip_verify", session.Origin.InsecureSkipVerify,
		"origin_websocket_upgrade_mode", session.Origin.WebsocketUpgradeMode,
		"ha_connections", session.HAConnections,
		"suggestion", "",
	)
}

func formatProtocol(protocol string) string {
	if protocol == "" {
		return "unknown"
	}
	return protocol
}

func buildSuggestion(finalErr error) string {
	if finalErr != nil {
		msg := finalErr.Error()
		if strings.Contains(msg, "rate limited") || strings.Contains(msg, "1015") {
			return "Retry later."
		}
	}
	return ""
}
