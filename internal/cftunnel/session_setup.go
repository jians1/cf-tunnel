package cftunnel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/api"
	tunnelconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/config"
	tunnelruntime "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/runtime"
	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

type quickTunnelReservationFunc func(ctx context.Context, runtimeConfig tunnelconfig.RuntimeConfig) (*api.QuickTunnelReservation, error)

type preparedSession struct {
	runtimeConfig tunnelconfig.RuntimeConfig
	reservation   *api.QuickTunnelReservation
	session       tunnelruntime.Session
}

func prepareQuickTunnelSession(
	ctx context.Context,
	cfg config.CFTunnelConfig,
	logger *slog.Logger,
) (*preparedSession, error) {
	return prepareQuickTunnelSessionWith(ctx, cfg, logger, func(ctx context.Context, runtimeConfig tunnelconfig.RuntimeConfig) (*api.QuickTunnelReservation, error) {
		client := api.NewClientWithOptions(runtimeConfig.QuickService, buildUserAgent(), api.ClientOptions{
			Timeout:       runtimeConfig.QuickServiceTimeout,
			RetryBackoffs: runtimeConfig.RetryBackoffs,
		})
		return client.CreateQuickTunnel(ctx)
	})
}

func prepareQuickTunnelSessionWith(
	ctx context.Context,
	cfg config.CFTunnelConfig,
	logger *slog.Logger,
	reserve quickTunnelReservationFunc,
) (*preparedSession, error) {
	runtimeConfig, err := tunnelconfig.Normalize(cfg)
	if err != nil {
		return nil, fmt.Errorf("normalize cftunnel config: %w", err)
	}

	reservation, err := reserve(ctx, runtimeConfig)
	if err != nil {
		if api.IsRateLimitedError(err) {
			return nil, fmt.Errorf(
				"quick tunnel API is rate limited right now; retry later: %w",
				err,
			)
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("quick tunnel request canceled or timed out: %w", err)
		}
		return nil, fmt.Errorf("create quick tunnel: %w", err)
	}

	session, err := tunnelruntime.BuildSession(runtimeConfig, reservation)
	if err != nil {
		return nil, fmt.Errorf("build cftunnel runtime session: %w", err)
	}

	return &preparedSession{
		runtimeConfig: runtimeConfig,
		reservation:   reservation,
		session:       session,
	}, nil
}
