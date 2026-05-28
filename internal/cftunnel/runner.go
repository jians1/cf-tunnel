package cftunnel

import (
	"context"
	"log/slog"
	"strings"
	"time"

	tunnelconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/config"
	tunnelruntime "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/runtime"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

type Runner struct {
	name   string
	cfg    config.CFTunnelConfig
	logger *slog.Logger
}

func NewRunner(cfg config.CFTunnelConfig, logger *slog.Logger) *Runner {
	return &Runner{
		name:   "cftunnel",
		cfg:    cfg,
		logger: logger.With("component", "cftunnel"),
	}
}

func NewNamedRunner(name string, cfg config.CFTunnelConfig, logger *slog.Logger) *Runner {
	tunnelName := name
	if strings.TrimSpace(tunnelName) == "" {
		tunnelName = "cftunnel"
	}
	return &Runner{
		name:   tunnelName,
		cfg:    cfg,
		logger: logger.With("component", "cftunnel", "tunnel_name", tunnelName),
	}
}

func (r *Runner) Name() string {
	return r.name
}

func (r *Runner) Run(ctx context.Context) error {
	prepared, err := prepareQuickTunnelSession(ctx, r.cfg, r.logger)
	if err != nil {
		return err
	}

	logQuickTunnelSummary(r.logger, formatProtocol(r.cfg.EdgeProtocol), prepared.session)

	bridge := tunnelruntime.NewBridgeRunner(prepared.session, r.logger)
	bridge.SetHTTP2Options(buildHTTP2ServerOptions(prepared.runtimeConfig, r.logger))
	bridge.SetQUICOptions(buildQUICRuntimeOptions(r.cfg, r.logger))
	return bridge.Run(ctx)
}

func buildUserAgent() string {
	return "cf-quicktunnel-ipv6pool/dev"
}

func buildHTTP2ServerOptions(_ tunnelconfig.RuntimeConfig, logger *slog.Logger) tunnelruntime.HTTP2ServerOptions {
	return tunnelruntime.HTTP2ServerOptions{
		EdgeAddressProvider: tunnelruntime.NewCloudflareEdgeAddressProvider("", tunnelruntime.EdgeIPAuto, logger),
		DialTimeout:         10 * time.Second,
	}
}

func buildQUICRuntimeOptions(_ config.CFTunnelConfig, logger *slog.Logger) tunnelruntime.QUICRuntimeOptions {
	return tunnelruntime.QUICRuntimeOptions{
		EdgeAddressProvider: tunnelruntime.NewCloudflareEdgeAddressProvider("", tunnelruntime.EdgeIPAuto, logger),
		DialTimeout:         10 * time.Second,
	}
}
