package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type Instance struct {
	Session         Session
	Prepared        *PreparedRuntime
	UpstreamBinding *UpstreamBinding
	HTTP2DialConfig *HTTP2DialConfig
	HTTP2Server     *HTTP2Server
	QUICDialConfig  *QUICDialConfig
	QUICRuntime     *QUICRuntime
}

type InstanceOptions struct {
	HTTP2 HTTP2ServerOptions
	QUIC  QUICRuntimeOptions
}

type QUICRuntimeOptions struct {
	DialAddress         string
	EdgeAddressProvider EdgeAddressProvider
	DialTimeout         time.Duration
	DialConfig          *QUICDialConfig
	ConnectedFuse       ConnectedFuse
	ConnIndex           uint8
}

func NewInstance(session Session, logger *slog.Logger) (*Instance, error) {
	return NewInstanceWithOptions(session, logger, HTTP2ServerOptions{})
}

func NewInstanceWithOptions(session Session, logger *slog.Logger, http2Options HTTP2ServerOptions) (*Instance, error) {
	return NewInstanceWithRuntimeOptions(session, logger, InstanceOptions{
		HTTP2: http2Options,
	})
}

func NewInstanceWithRuntimeOptions(session Session, logger *slog.Logger, options InstanceOptions) (*Instance, error) {
	http2Options := options.HTTP2
	edgeProtocol, err := ParseEdgeProtocol(session.Edge.Protocol)
	if err != nil {
		return nil, fmt.Errorf("resolve edge protocol before building runtime instance: %w", err)
	}

	prepared, err := PrepareRuntime(session)
	if err != nil {
		return nil, fmt.Errorf("prepare runtime: %w", err)
	}

	adapter := NewUpstreamAdapter()
	binding, err := adapter.Bind(session)
	if err != nil {
		return nil, fmt.Errorf("bind upstream runtime: %w", err)
	}

	instance := &Instance{
		Session:         session,
		Prepared:        prepared,
		UpstreamBinding: binding,
	}

	if edgeProtocol == EdgeProtocol(edgeProtocolHTTP2) {
		http2Options.TunnelProperties = binding.TunnelProperties
		if http2Options.DialConfig == nil && (http2Options.DialAddress != "" || http2Options.EdgeAddressProvider != nil) {
			dialConfig, err := NewHTTP2DialConfigWithProvider(
				prepared,
				http2Options.DialAddress,
				http2Options.EdgeAddressProvider,
				http2Options.DialTimeout,
			)
			if err != nil {
				return nil, fmt.Errorf("build http2 dial config: %w", err)
			}
			http2Options.DialConfig = dialConfig
		}
		instance.HTTP2DialConfig = http2Options.DialConfig
		server, err := NewHTTP2ServerWithHandler(prepared, logger, prepared.OriginProxy, http2Options)
		if err != nil {
			return nil, fmt.Errorf("build http2 server: %w", err)
		}
		instance.HTTP2Server = server
	}

	if edgeProtocol == EdgeProtocol(edgeProtocolQUIC) {
		if options.QUIC.DialConfig == nil && (options.QUIC.DialAddress != "" || options.QUIC.EdgeAddressProvider != nil) {
			dialConfig, err := NewQUICDialConfigWithProvider(
				prepared,
				options.QUIC.DialAddress,
				options.QUIC.EdgeAddressProvider,
				options.QUIC.DialTimeout,
			)
			if err != nil {
				return nil, fmt.Errorf("build quic dial config: %w", err)
			}
			options.QUIC.DialConfig = dialConfig
		}
		instance.QUICDialConfig = options.QUIC.DialConfig
		quicRuntime, err := NewQUICRuntimeWithOptions(session, logger, options.QUIC)
		if err != nil {
			return nil, fmt.Errorf("build quic runtime: %w", err)
		}
		instance.QUICRuntime = quicRuntime
	}

	return instance, nil
}

func (i *Instance) Run(ctx context.Context) error {
	if i == nil {
		return fmt.Errorf("nil runtime instance")
	}
	if i.HTTP2Server != nil {
		return i.HTTP2Server.Serve(ctx)
	}
	if i.QUICRuntime != nil {
		return i.QUICRuntime.Run(ctx)
	}

	return fmt.Errorf("runtime instance has no runnable transport")
}
