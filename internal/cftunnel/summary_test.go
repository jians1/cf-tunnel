package cftunnel

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/jians1/cf-tunnel/internal/cftunnel/origin"
	tunnelruntime "github.com/jians1/cf-tunnel/internal/cftunnel/runtime"
)

func TestLogQuickTunnelSummaryKeepsInfoOutputCompact(t *testing.T) {
	t.Parallel()

	output := captureTunnelSummary(t, slog.LevelInfo, quickTunnelSession())

	assertLogContains(t, output,
		`"msg":"quick tunnel ready"`,
		`"url":"https://demo.trycloudflare.com"`,
		`"protocol":"quic"`,
		`"origin":"http://127.0.0.1:8080"`,
	)
	assertLogOmits(t, output,
		"mode",
		"quick_tunnel_hostname",
		"origin_server_name",
		"origin_insecure_skip_verify",
		"origin_websocket_upgrade_mode",
		"ha_connections",
		"suggestion",
	)
}

func TestLogQuickTunnelSummaryKeepsVerboseFieldsAtDebug(t *testing.T) {
	t.Parallel()

	output := captureTunnelSummary(t, slog.LevelDebug, quickTunnelSession())

	assertLogContains(t, output,
		`"msg":"quick tunnel detail"`,
		`"hostname":"demo.trycloudflare.com"`,
		`"origin_protocol":"http"`,
		`"origin_server_name":"origin.internal"`,
		`"origin_insecure_skip_verify":true`,
		`"origin_websocket_upgrade_mode":true`,
		`"ha_connections":4`,
	)
}

func TestLogFormalTunnelSummaryKeepsInfoOutputCompact(t *testing.T) {
	t.Parallel()

	session := quickTunnelSession()
	session.QuickTunnel = false

	output := captureTunnelSummary(t, slog.LevelInfo, session)

	assertLogContains(t, output,
		`"msg":"cloudflare tunnel ready"`,
		`"tunnel_id":"11111111-1111-1111-1111-111111111111"`,
		`"protocol":"quic"`,
		`"origin":"http://127.0.0.1:8080"`,
	)
	assertLogOmits(t, output,
		"mode",
		"origin_server_name",
		"origin_insecure_skip_verify",
		"origin_websocket_upgrade_mode",
		"ha_connections",
	)
}

func captureTunnelSummary(t *testing.T, level slog.Level, session tunnelruntime.Session) string {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level}))
	logTunnelSummary(logger, "quic", session)
	return buf.String()
}

func quickTunnelSession() tunnelruntime.Session {
	return tunnelruntime.Session{
		TunnelID:  "11111111-1111-1111-1111-111111111111",
		Hostname:  "demo.trycloudflare.com",
		PublicURL: "https://demo.trycloudflare.com",
		Origin: tunnelruntime.OriginSettings{
			URL:                  "http://127.0.0.1:8080",
			Protocol:             origin.ProtocolHTTP,
			ServerName:           "origin.internal",
			InsecureSkipVerify:   true,
			WebsocketUpgradeMode: true,
		},
		QuickTunnel:   true,
		HAConnections: 4,
	}
}

func assertLogContains(t *testing.T, output string, snippets ...string) {
	t.Helper()

	for _, snippet := range snippets {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected log to contain %q; got %s", snippet, output)
		}
	}
}

func assertLogOmits(t *testing.T, output string, snippets ...string) {
	t.Helper()

	for _, snippet := range snippets {
		if strings.Contains(output, snippet) {
			t.Fatalf("expected log to omit %q; got %s", snippet, output)
		}
	}
}
