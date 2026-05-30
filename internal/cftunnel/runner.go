package cftunnel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	tunnelconfig "github.com/jians1/cf-tunnel/internal/cftunnel/config"
	tunnelruntime "github.com/jians1/cf-tunnel/internal/cftunnel/runtime"
	"github.com/jians1/cf-tunnel/internal/config"
	"github.com/jians1/cf-tunnel/internal/health"
)

type Runner struct {
	name      string
	cfg       config.CFTunnelConfig
	logger    *slog.Logger
	readiness *tunnelReadiness
}

type tunnelReadiness struct {
	mu     sync.RWMutex
	name   string
	status string
	onSet  func(name, status string)
}

func newTunnelReadiness(name string) *tunnelReadiness {
	return &tunnelReadiness{name: name, status: "pending"}
}

func newTunnelReadinessWithCallback(name string, onSet func(name, status string)) *tunnelReadiness {
	return &tunnelReadiness{name: name, status: "pending", onSet: onSet}
}

func (r *tunnelReadiness) set(status string) {
	r.mu.Lock()
	r.status = status
	name := r.name
	onSet := r.onSet
	r.mu.Unlock()
	if onSet != nil {
		onSet(name, status)
	}
}

func (r *tunnelReadiness) snapshot() (name, status string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.name, r.status
}

func (r *tunnelReadiness) Connected() {
	r.set("ready")
}

func (r *tunnelReadiness) IsConnected() bool {
	_, status := r.snapshot()
	return status == "ready"
}

func NewRunner(cfg config.CFTunnelConfig, logger *slog.Logger) *Runner {
	const name = "cftunnel"
	return &Runner{
		name:      name,
		cfg:       cfg,
		logger:    logger.With("component", "cftunnel"),
		readiness: newTunnelReadiness(name),
	}
}

func NewNamedRunner(name string, cfg config.CFTunnelConfig, logger *slog.Logger) *Runner {
	return newNamedRunnerWithReadiness(name, cfg, logger, nil)
}

func newNamedRunnerWithReadiness(name string, cfg config.CFTunnelConfig, logger *slog.Logger, readiness *tunnelReadiness) *Runner {
	tunnelName := name
	if strings.TrimSpace(tunnelName) == "" {
		tunnelName = "cftunnel"
	}
	if readiness == nil {
		readiness = newTunnelReadiness(tunnelName)
	}
	return &Runner{
		name:      tunnelName,
		cfg:       cfg,
		logger:    logger.With("component", "cftunnel", "tunnel_name", tunnelName),
		readiness: readiness,
	}
}

func (r *Runner) Name() string {
	return r.name
}

func (r *Runner) Run(ctx context.Context) error {
	r.readiness.set("starting")
	prepared, err := prepareTunnelSession(ctx, r.cfg, r.logger)
	if err != nil {
		r.readiness.set(statusForRunError(ctx, err))
		return err
	}

	logTunnelSummary(r.logger, formatProtocol(r.cfg.EdgeProtocol), prepared.session)

	bridge := tunnelruntime.NewBridgeRunner(prepared.session, r.logger)
	bridge.SetHTTP2Options(buildHTTP2ServerOptions(prepared.runtimeConfig, r.logger, r.readiness))
	bridge.SetQUICOptions(buildQUICRuntimeOptions(r.cfg, r.logger, r.readiness))
	err = bridge.Run(ctx)
	r.readiness.set(statusForRunError(ctx, err))
	return err
}

func (r *Runner) ReadyStatus() health.ReadyStatus {
	name, status := r.readiness.snapshot()
	ready := status == "ready"
	readyCount := 0
	if ready {
		readyCount = 1
	}
	failed := 0
	if status == "failed" {
		failed = 1
	}
	return health.ReadyStatus{
		Ready: ready,
		Summary: fmt.Sprintf(
			"mode=single total=1 ready=%d failed=%d details=[%s:%s]",
			readyCount,
			failed,
			name,
			status,
		),
	}
}

func (r *Runner) markReadyForTest() {
	r.readiness.Connected()
}

func statusForRunError(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return "stopped"
	}
	if err != nil {
		return "failed"
	}
	return "exited"
}

func buildUserAgent() string {
	return "cf-tunnel/dev"
}

func buildHTTP2ServerOptions(_ tunnelconfig.RuntimeConfig, logger *slog.Logger, connected tunnelruntime.ConnectedFuse) tunnelruntime.HTTP2ServerOptions {
	return tunnelruntime.HTTP2ServerOptions{
		EdgeAddressProvider: tunnelruntime.NewCloudflareEdgeAddressProvider("", tunnelruntime.EdgeIPAuto, logger),
		DialTimeout:         10 * time.Second,
		ConnectedFuse:       connected,
	}
}

func buildQUICRuntimeOptions(_ config.CFTunnelConfig, logger *slog.Logger, connected tunnelruntime.ConnectedFuse) tunnelruntime.QUICRuntimeOptions {
	return tunnelruntime.QUICRuntimeOptions{
		EdgeAddressProvider: tunnelruntime.NewCloudflareEdgeAddressProvider("", tunnelruntime.EdgeIPAuto, logger),
		DialTimeout:         10 * time.Second,
		ConnectedFuse:       connected,
	}
}
