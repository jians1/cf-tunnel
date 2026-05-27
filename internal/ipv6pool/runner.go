package ipv6pool

import (
	"context"
	"log/slog"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
	"golang.org/x/sync/errgroup"
)

type Runner struct {
	cfg    config.IPv6PoolConfig
	logger *slog.Logger
}

func NewRunner(cfg config.IPv6PoolConfig, logger *slog.Logger) *Runner {
	return &Runner{
		cfg:    cfg,
		logger: logger.With("component", "ipv6pool"),
	}
}

func (r *Runner) Name() string {
	return "ipv6pool"
}

func (r *Runner) Run(ctx context.Context) error {
	pool, err := NewPool(r.cfg.CIDR, r.cfg.BindInterface, r.cfg.Strategy)
	if err != nil {
		return err
	}

	dialer := NewDialer(pool)
	group, groupCtx := errgroup.WithContext(ctx)

	if r.cfg.HTTPListen != "" {
		httpProxy := NewHTTPProxy(r.cfg.HTTPListen, dialer, r.logger)
		group.Go(func() error {
			return httpProxy.Run(groupCtx)
		})
	}

	if r.cfg.SOCKS5Listen != "" {
		socksProxy := NewSOCKS5Proxy(r.cfg.SOCKS5Listen, dialer, r.logger)
		group.Go(func() error {
			return socksProxy.Run(groupCtx)
		})
	}

	r.logger.Info(
		"ipv6pool runner started",
		"http_listen", r.cfg.HTTPListen,
		"socks5_listen", r.cfg.SOCKS5Listen,
		"bind_interface", r.cfg.BindInterface,
		"cidr", r.cfg.CIDR,
		"strategy", r.cfg.Strategy,
	)

	return group.Wait()
}
