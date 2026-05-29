package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Options struct {
	ShutdownTimeout time.Duration
}

type Runner interface {
	Name() string
	Run(context.Context) error
}

func Run(ctx context.Context, logger *slog.Logger, runners ...Runner) error {
	return RunWithOptions(ctx, logger, Options{}, runners...)
}

func RunWithOptions(ctx context.Context, logger *slog.Logger, opts Options, runners ...Runner) error {
	if len(runners) == 0 {
		return errors.New("no runners configured")
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	groupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type runnerResult struct {
		name string
		err  error
	}
	results := make(chan runnerResult, len(runners))
	for _, runner := range runners {
		r := runner
		go func() {
			logger.Info("starting runner", "runner", r.Name())
			err := r.Run(groupCtx)
			if err != nil && !errors.Is(err, context.Canceled) {
				err = fmt.Errorf("%s: %w", r.Name(), err)
				cancel()
			}
			logger.Info("runner stopped", "runner", r.Name())
			results <- runnerResult{name: r.Name(), err: err}
		}()
	}

	timerStarted := false
	var timeout <-chan time.Time
	var firstErr error
	completed := 0
	for completed < len(runners) {
		select {
		case result := <-results:
			completed++
			if firstErr == nil && result.err != nil {
				firstErr = result.err
			}
		case <-ctx.Done():
			if opts.ShutdownTimeout <= 0 {
				continue
			}
			if !timerStarted {
				timeout = time.After(opts.ShutdownTimeout)
				timerStarted = true
			}
		case <-timeout:
			return fmt.Errorf("shutdown timeout after %s", opts.ShutdownTimeout)
		}
	}
	return firstErr
}
