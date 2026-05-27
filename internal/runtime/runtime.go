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

	"golang.org/x/sync/errgroup"
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

	group, groupCtx := errgroup.WithContext(ctx)
	for _, runner := range runners {
		r := runner
		group.Go(func() error {
			logger.Info("starting runner", "runner", r.Name())
			if err := r.Run(groupCtx); err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("%s: %w", r.Name(), err)
			}
			logger.Info("runner stopped", "runner", r.Name())
			return nil
		})
	}

	done := make(chan error, 1)
	go func() {
		done <- group.Wait()
	}()

	if opts.ShutdownTimeout <= 0 {
		return <-done
	}

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		select {
		case err := <-done:
			return err
		case <-time.After(opts.ShutdownTimeout):
			return fmt.Errorf("shutdown timeout after %s", opts.ShutdownTimeout)
		}
	}
}
