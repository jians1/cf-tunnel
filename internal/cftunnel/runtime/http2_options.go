package runtime

import (
	"context"
	"io"
	"net"
	"time"

	tunnelpogs "github.com/cloudflare/cloudflared/tunnelrpc/pogs"
	"github.com/google/uuid"
)

type HTTP2ServerOptions struct {
	ControlStreamHandler   ControlStreamHandler
	RegistrationClientFunc registrationClientFactory
	LocalEdgeDriver        bool
	DialAddress            string
	EdgeAddressProvider    EdgeAddressProvider
	DialTimeout            time.Duration
	DialConfig             *HTTP2DialConfig
	TransportFactory       HTTP2TransportFactory
	TunnelProperties       *RuntimeTunnelProperties
	GracefulShutdownC      <-chan struct{}
	GracePeriod            time.Duration
	RegisterTimeout        time.Duration
	ConnectedFuse          ConnectedFuse
	ConnIndex              uint8
	EdgeAddress            net.IP
}

func (o HTTP2ServerOptions) withDefaults() HTTP2ServerOptions {
	if o.GracePeriod == 0 {
		o.GracePeriod = time.Second
	}
	if o.RegisterTimeout == 0 {
		o.RegisterTimeout = time.Second
	}
	if o.ConnectedFuse == nil {
		o.ConnectedFuse = noopConnectedFuse{}
	}
	if o.RegistrationClientFunc == nil {
		o.RegistrationClientFunc = newRegistrationClient
	}
	if o.TransportFactory == nil {
		if o.DialConfig != nil {
			factory, err := o.DialConfig.TransportFactory()
			if err == nil {
				o.TransportFactory = factory
			}
		}
		if o.TransportFactory == nil {
			o.TransportFactory = PipeHTTP2TransportFactory{}
		}
	}
	return o
}

type noopConnectedFuse struct{}

func (noopConnectedFuse) Connected()        {}
func (noopConnectedFuse) IsConnected() bool { return false }

type mockNamedTunnelRPCClient struct {
	shouldFail   error
	registered   chan struct{}
	unregistered chan struct{}
}

func (mc mockNamedTunnelRPCClient) SendLocalConfiguration(context.Context, []byte) error {
	return nil
}

func (mc mockNamedTunnelRPCClient) RegisterConnection(
	context.Context,
	tunnelpogs.TunnelAuth,
	[16]byte,
	*tunnelpogs.ConnectionOptions,
	uint8,
	net.IP,
) (*tunnelpogs.ConnectionDetails, error) {
	if mc.shouldFail != nil {
		return nil, mc.shouldFail
	}
	close(mc.registered)
	return &tunnelpogs.ConnectionDetails{
		Location:                "LIS",
		UUID:                    uuid.New(),
		TunnelIsRemotelyManaged: false,
	}, nil
}

func (mc mockNamedTunnelRPCClient) GracefulShutdown(context.Context, time.Duration) error {
	close(mc.unregistered)
	return nil
}

func (mockNamedTunnelRPCClient) Close() {}

type mockRPCClientFactory struct {
	shouldFail   error
	registered   chan struct{}
	unregistered chan struct{}
}

func (mf *mockRPCClientFactory) newMockRPCClient(context.Context, io.ReadWriteCloser, time.Duration) registrationClient {
	return &mockNamedTunnelRPCClient{
		shouldFail:   mf.shouldFail,
		registered:   mf.registered,
		unregistered: mf.unregistered,
	}
}
