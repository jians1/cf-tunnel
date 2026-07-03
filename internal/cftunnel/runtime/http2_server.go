package runtime

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

type HTTP2Server struct {
	connection interface {
		Serve(context.Context) error
	}
	edgeConn net.Conn
	closeFn  func() error
}

func NewHTTP2Server(prepared *PreparedRuntime, logger *slog.Logger) (*HTTP2Server, error) {
	return NewHTTP2ServerWithHandler(prepared, logger, prepared.OriginProxy, HTTP2ServerOptions{})
}

func NewHTTP2ServerWithHandler(prepared *PreparedRuntime, logger *slog.Logger, handler http.Handler, options HTTP2ServerOptions) (*HTTP2Server, error) {
	if prepared == nil {
		return nil, io.ErrUnexpectedEOF
	}
	if handler == nil {
		return nil, io.ErrUnexpectedEOF
	}
	options = options.withDefaults()

	upstreamOriginProxy := NewUpstreamOriginProxy(handler)
	orchestrator, err := NewUpstreamOrchestrator(upstreamOriginProxy, prepared.Session)
	if err != nil {
		return nil, err
	}

	transport, err := options.TransportFactory.NewTransport(context.Background())
	if err != nil {
		return nil, err
	}
	edgeConn := transport.EdgeConn()
	cfdConn := transport.ServerConn()
	if cfdConn == nil {
		cfdConn = edgeConn
		edgeConn = nil
	}
	if cfdConn == nil {
		_ = transport.Close()
		return nil, fmt.Errorf("http2 transport must provide a server conn")
	}
	connOptions, err := newRuntimeConnectionOptions(options.ConnectorID)
	if err != nil {
		_ = transport.Close()
		return nil, fmt.Errorf("build http2 connection options: %w", err)
	}
	zlog := newZeroLoggerFromSlog(logger)
	controlStreamHandler := options.ControlStreamHandler
	if controlStreamHandler == nil {
		if options.RegistrationClientFunc != nil && options.TunnelProperties != nil {
			controlStreamHandler = NewControlStream(runtimeControlStreamOptions{
				options.ConnectedFuse,
				options.TunnelProperties,
				options.ConnIndex,
				options.EdgeAddress,
				func(ctx context.Context, rw io.ReadWriteCloser, timeout time.Duration) registrationClient {
					return options.RegistrationClientFunc(ctx, rw, timeout)
				},
				options.RegisterTimeout,
				options.GracefulShutdownC,
				options.GracePeriod,
			})
		} else {
			controlStreamHandler = fakeControlStreamHandler{}
		}
	}

	http2Conn := NewRuntimeHTTP2Connection(
		cfdConn,
		orchestrator,
		connOptions,
		options.ConnIndex,
		controlStreamHandler,
		&zlog,
	)

	return &HTTP2Server{
		connection: http2Conn,
		edgeConn:   edgeConn,
		closeFn:    transport.Close,
	}, nil
}

func (s *HTTP2Server) Serve(ctx context.Context) error {
	if s.edgeConn != nil {
		defer s.edgeConn.Close()
	}
	if s.closeFn != nil {
		defer s.closeFn()
	}
	return s.connection.Serve(ctx)
}

func (s *HTTP2Server) EdgeConn() net.Conn {
	return s.edgeConn
}

type fakeControlStreamHandler struct{}

func (fakeControlStreamHandler) ServeControlStream(_ context.Context, _ io.ReadWriteCloser, _ *runtimeConnectionOptions, _ TunnelConfigJSONGetter) error {
	return nil
}

func (fakeControlStreamHandler) IsStopped() bool {
	return true
}
