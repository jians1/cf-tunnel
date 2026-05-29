package main

import (
	"context"
	"fmt"
	"os"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/health"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/logging"
	appRuntime "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/runtime"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(2)
	}

	logger := logging.New(cfg.LogLevel, cfg.LogFormat)
	ctx := context.Background()

	var runners []appRuntime.Runner
	var healthRunner *health.Runner
	if cfg.HealthListen != "" {
		healthRunner = health.NewRunner(cfg.HealthListen, logger)
		runners = append(runners, healthRunner)
	}
	if len(cfg.Tunnels) > 0 {
		multi, err := cftunnel.NewMultiRunner(cfg.Tunnels, logger)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config error: %v\n", err)
			os.Exit(2)
		}
		runners = append(runners, multi)
	} else {
		runners = append(runners, cftunnel.NewRunner(cfg.CFTunnel, logger))
	}

	if err := appRuntime.RunWithOptions(ctx, logger, appRuntime.Options{
		ShutdownTimeout: cfg.ShutdownTimeout,
	}, runners...); err != nil {
		logger.Error("application stopped with error", "error", err)
		os.Exit(1)
	}
}
