package cftunnel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jians1/cf-tunnel/internal/config"
	"github.com/jians1/cf-tunnel/internal/health"
)

const multiTunnelStartInterval = 5 * time.Second

type MultiRunner struct {
	runners       []tunnelRunner
	logger        *slog.Logger
	startInterval time.Duration
	mu            sync.RWMutex
	state         map[string]string
	status        map[string]health.TunnelStatus
}

type tunnelRunner interface {
	Name() string
	Run(context.Context) error
}

func NewMultiRunner(tunnels []config.NamedTunnelConfig, logger *slog.Logger) (*MultiRunner, error) {
	if len(tunnels) == 0 {
		return nil, errors.New("no tunnel instances configured")
	}

	multi := &MultiRunner{
		logger:        logger.With("component", "cftunnel"),
		startInterval: multiTunnelStartInterval,
		state:         make(map[string]string, len(tunnels)),
		status:        make(map[string]health.TunnelStatus, len(tunnels)),
	}
	for _, tunnel := range tunnels {
		name := tunnel.Name
		if strings.TrimSpace(name) == "" {
			name = "cftunnel"
		}
		readiness := newTunnelReadinessWithCallback(name, multi.setState)
		multi.runners = append(multi.runners, newNamedRunnerWithReadiness(name, tunnel.CFTunnel, logger, readiness))
	}
	return multi, nil
}

func (r *MultiRunner) Name() string {
	return "cftunnel-multi"
}

func (r *MultiRunner) Run(ctx context.Context) error {
	if len(r.runners) == 0 {
		return errors.New("no tunnel runners configured")
	}

	groupCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var runErrs []error

	for i, runner := range r.runners {
		tunnelRunner := runner
		r.setState(tunnelRunner.Name(), "starting")
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.logger.Info("starting tunnel instance", "tunnel_name", tunnelRunner.Name())
			if err := tunnelRunner.Run(groupCtx); err != nil && !errors.Is(err, context.Canceled) {
				r.setState(tunnelRunner.Name(), "failed")
				mu.Lock()
				runErrs = append(runErrs, fmt.Errorf("tunnel %s: %w", tunnelRunner.Name(), err))
				mu.Unlock()
				cancel()
				return
			}
			if errors.Is(groupCtx.Err(), context.Canceled) {
				r.setState(tunnelRunner.Name(), "stopped")
			} else {
				r.setState(tunnelRunner.Name(), "exited")
			}
			r.logger.Info("tunnel instance stopped", "tunnel_name", tunnelRunner.Name())
		}()
		if i < len(r.runners)-1 && r.startInterval > 0 {
			if err := waitForStartInterval(groupCtx, r.startInterval); err != nil {
				break
			}
		}
	}
	wg.Wait()

	if len(runErrs) == 0 {
		return nil
	}
	return joinTunnelErrors(runErrs)
}

func waitForStartInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func joinTunnelErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	msg := make([]string, 0, len(errs))
	for _, err := range errs {
		msg = append(msg, err.Error())
	}
	return fmt.Errorf("multiple tunnel failures (%d): %s", len(errs), strings.Join(msg, "; "))
}

func (r *MultiRunner) setState(name, status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == nil {
		r.state = map[string]string{}
	}
	if r.status == nil {
		r.status = map[string]health.TunnelStatus{}
	}
	r.state[name] = status
	snapshot := r.status[name]
	snapshot.Name = name
	snapshot.Status = status
	r.status[name] = snapshot
}

func (r *MultiRunner) ReadinessSummary() string {
	return r.ReadyStatus().Summary
}

func (r *MultiRunner) ReadyStatus() health.ReadyStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.runners) == 0 {
		return health.ReadyStatus{Ready: false, Summary: "mode=multi total=0 ready=0 failed=0 details=[]"}
	}

	total := len(r.runners)
	readyCount := 0
	failed := 0
	details := make([]string, 0, total)
	for _, runner := range r.runners {
		name := runner.Name()
		status := r.state[name]
		if status == "" {
			status = "pending"
		}
		if status == "ready" {
			readyCount++
		}
		if status == "failed" {
			failed++
		}
		details = append(details, fmt.Sprintf("%s:%s", name, status))
	}
	return health.ReadyStatus{
		Ready: readyCount == total,
		Summary: fmt.Sprintf(
			"mode=multi total=%d ready=%d failed=%d details=[%s]",
			total,
			readyCount,
			failed,
			strings.Join(details, ","),
		),
	}
}

func (r *MultiRunner) Status() health.StatusPayload {
	ready := r.ReadyStatus()
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.runners))
	for _, runner := range r.runners {
		names = append(names, runner.Name())
	}
	sort.Strings(names)

	tunnels := make([]health.TunnelStatus, 0, len(names))
	for _, name := range names {
		snapshot := r.status[name]
		if snapshot.Name == "" {
			snapshot.Name = name
		}
		if snapshot.Status == "" {
			snapshot.Status = "pending"
		}
		tunnels = append(tunnels, snapshot)
	}

	return health.StatusPayload{
		Mode:    "multi",
		Ready:   ready.Ready,
		Summary: ready.Summary,
		Tunnels: tunnels,
	}
}
