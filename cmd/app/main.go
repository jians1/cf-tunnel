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
	if cfg.HealthListen != "" {
		runners = append(runners, health.NewRunner(cfg.HealthListen, logger))
	}
	if cfg.CFTunnel.Enabled {
		runners = append(runners, cftunnel.NewRunner(cfg.CFTunnel, logger))
	}

	if err := appRuntime.RunWithOptions(ctx, logger, appRuntime.Options{
		ShutdownTimeout: cfg.ShutdownTimeout,
	}, runners...); err != nil {
		logger.Error("application stopped with error", "error", err)
		os.Exit(1)
	}
}
