package cftunnel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

type MultiRunner struct {
	runners []tunnelRunner
	logger  *slog.Logger
	mu      sync.RWMutex
	state   map[string]string
}

type tunnelRunner interface {
	Name() string
	Run(context.Context) error
}

func NewMultiRunner(tunnels []config.NamedTunnelConfig, logger *slog.Logger) (*MultiRunner, error) {
	if len(tunnels) == 0 {
		return nil, errors.New("no tunnel instances configured")
	}

	runners := make([]tunnelRunner, 0, len(tunnels))
	for _, tunnel := range tunnels {
		runners = append(runners, NewNamedRunner(tunnel.Name, tunnel.CFTunnel, logger))
	}
	return &MultiRunner{
		runners: runners,
		logger:  logger.With("component", "cftunnel"),
		state:   make(map[string]string, len(runners)),
	}, nil
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

	for _, runner := range r.runners {
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
	}
	wg.Wait()

	if len(runErrs) == 0 {
		return nil
	}
	return joinTunnelErrors(runErrs)
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
	r.state[name] = status
}

func (r *MultiRunner) ReadinessSummary() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.runners) == 0 {
		return "mode=multi total=0 running=0 failed=0 details=[]"
	}

	total := len(r.runners)
	running := 0
	failed := 0
	details := make([]string, 0, total)
	for _, runner := range r.runners {
		name := runner.Name()
		status := r.state[name]
		if status == "" {
			status = "pending"
		}
		if status == "starting" {
			running++
		}
		if status == "failed" {
			failed++
		}
		details = append(details, fmt.Sprintf("%s:%s", name, status))
	}
	return fmt.Sprintf("mode=multi total=%d running=%d failed=%d details=[%s]", total, running, failed, strings.Join(details, ","))
}
