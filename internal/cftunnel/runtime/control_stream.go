package runtime

import (
	"context"
	"io"
	"net"
	"time"
)

type runtimeControlStreamOptions struct {
	ConnectedFuse      ConnectedFuse
	TunnelProperties   *RuntimeTunnelProperties
	ConnIndex          uint8
	EdgeAddress        net.IP
	RegisterClientFunc registrationClientFactory
	RegisterTimeout    time.Duration
	GracefulShutdownC  <-chan struct{}
	GracePeriod        time.Duration
}

type runtimeControlStream struct {
	connectedFuse      ConnectedFuse
	tunnelProperties   *RuntimeTunnelProperties
	connIndex          uint8
	edgeAddress        net.IP
	registerClientFunc registrationClientFactory
	registerTimeout    time.Duration
	gracefulShutdownC  <-chan struct{}
	gracePeriod        time.Duration
	stoppedGracefully  bool
}

func NewControlStream(opts runtimeControlStreamOptions) ControlStreamHandler {
	if opts.ConnectedFuse == nil {
		opts.ConnectedFuse = noopConnectedFuse{}
	}
	if opts.RegisterClientFunc == nil {
		opts.RegisterClientFunc = newRegistrationClient
	}
	return &runtimeControlStream{
		connectedFuse:      opts.ConnectedFuse,
		tunnelProperties:   opts.TunnelProperties,
		connIndex:          opts.ConnIndex,
		edgeAddress:        opts.EdgeAddress,
		registerClientFunc: opts.RegisterClientFunc,
		registerTimeout:    opts.RegisterTimeout,
		gracefulShutdownC:  opts.GracefulShutdownC,
		gracePeriod:        opts.GracePeriod,
	}
}

func (c *runtimeControlStream) ServeControlStream(
	ctx context.Context,
	rw io.ReadWriteCloser,
	connOptions *runtimeConnectionOptions,
	tunnelConfigGetter TunnelConfigJSONGetter,
) error {
	registrationClient := c.registerClientFunc(ctx, rw, c.registerTimeout)
	registrationDetails, err := registrationClient.RegisterConnection(
		ctx,
		c.tunnelProperties.Credentials.Auth(),
		c.tunnelProperties.Credentials.TunnelID,
		connOptions,
		c.connIndex,
		c.edgeAddress,
	)
	if err != nil {
		defer registrationClient.Close()
		return err
	}
	c.connectedFuse.Connected()

	if c.connIndex == 0 && !registrationDetails.TunnelIsRemotelyManaged {
		if tunnelConfig, err := tunnelConfigGetter.GetConfigJSON(); err == nil {
			_ = registrationClient.SendLocalConfiguration(ctx, tunnelConfig)
		}
	}
	return c.waitForUnregister(ctx, registrationClient)
}

func (c *runtimeControlStream) waitForUnregister(ctx context.Context, registrationClient registrationClient) error {
	defer registrationClient.Close()
	var shutdownError error
	select {
	case <-ctx.Done():
		shutdownError = ctx.Err()
	case <-c.gracefulShutdownC:
		c.stoppedGracefully = true
	}
	if err := registrationClient.GracefulShutdown(ctx, c.gracePeriod); err != nil {
		return err
	}
	return shutdownError
}

func (c *runtimeControlStream) IsStopped() bool {
	return c.stoppedGracefully
}
