package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"flag"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/health"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/logging"
	appRuntime "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/runtime"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(2)
	}

	logger := logging.New(cfg.LogLevel, cfg.LogFormat)
	ctx := context.Background()

	runners, err := buildRunners(cfg, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(2)
	}

	if err := appRuntime.RunWithOptions(ctx, logger, appRuntime.Options{
		ShutdownTimeout: cfg.ShutdownTimeout,
	}, runners...); err != nil {
		logger.Error("application stopped with error", "error", err)
		os.Exit(1)
	}
}

func buildRunners(cfg config.AppConfig, logger *slog.Logger) ([]appRuntime.Runner, error) {
	var runners []appRuntime.Runner
	var healthRunner *health.Runner
	if cfg.HealthListen != "" {
		healthRunner = health.NewRunner(cfg.HealthListen, logger)
		runners = append(runners, healthRunner)
	}
	if len(cfg.Tunnels) > 0 {
		multi, err := cftunnel.NewMultiRunner(cfg.Tunnels, logger)
		if err != nil {
			return nil, err
		}
		if healthRunner != nil {
			healthRunner.SetReadyProvider(multi.ReadyStatus)
		}
		runners = append(runners, multi)
		return runners, nil
	}
	tunnelRunner := cftunnel.NewRunner(cfg.CFTunnel, logger)
	if healthRunner != nil {
		healthRunner.SetReadyProvider(tunnelRunner.ReadyStatus)
	}
	runners = append(runners, tunnelRunner)
	return runners, nil
}
