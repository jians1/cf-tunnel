package runtime

import (
	"context"
	"io"
	"net"
	"time"

	tunnelpogs "github.com/cloudflare/cloudflared/tunnelrpc/pogs"
	"zombiezen.com/go/capnproto2/rpc"
)

type registrationClient interface {
	RegisterConnection(
		ctx context.Context,
		auth runtimeTunnelAuth,
		tunnelID [16]byte,
		options *runtimeConnectionOptions,
		connIndex uint8,
		edgeAddress net.IP,
	) (*runtimeConnectionDetails, error)
	SendLocalConfiguration(ctx context.Context, config []byte) error
	GracefulShutdown(ctx context.Context, gracePeriod time.Duration) error
	Close()
}

type registrationClientFactory func(context.Context, io.ReadWriteCloser, time.Duration) registrationClient

type runtimeRegistrationClient struct {
	client         tunnelpogs.RegistrationServer_PogsClient
	transport      rpc.Transport
	requestTimeout time.Duration
}

func newRegistrationClient(ctx context.Context, stream io.ReadWriteCloser, requestTimeout time.Duration) registrationClient {
	transport := safeTransport(stream)
	conn := newClientConn(transport)
	client := tunnelpogs.NewRegistrationServer_PogsClient(conn.Bootstrap(ctx), conn)
	return &runtimeRegistrationClient{
		client:         client,
		transport:      transport,
		requestTimeout: requestTimeout,
	}
}

func (r *runtimeRegistrationClient) RegisterConnection(
	ctx context.Context,
	auth runtimeTunnelAuth,
	tunnelID [16]byte,
	options *runtimeConnectionOptions,
	connIndex uint8,
	edgeAddress net.IP,
) (*runtimeConnectionDetails, error) {
	ctx, cancel := context.WithTimeout(ctx, r.requestTimeout)
	defer cancel()
	return r.client.RegisterConnection(ctx, auth, tunnelID, connIndex, options)
}

func (r *runtimeRegistrationClient) SendLocalConfiguration(ctx context.Context, config []byte) error {
	ctx, cancel := context.WithTimeout(ctx, r.requestTimeout)
	defer cancel()
	return r.client.SendLocalConfiguration(ctx, config)
}

func (r *runtimeRegistrationClient) GracefulShutdown(ctx context.Context, gracePeriod time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, gracePeriod)
	defer cancel()
	return r.client.UnregisterConnection(ctx)
}

func (r *runtimeRegistrationClient) Close() {
	_ = r.client.Close()
	_ = r.transport.Close()
}
