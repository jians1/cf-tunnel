package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	tunnelpogs "github.com/cloudflare/cloudflared/tunnelrpc/pogs"
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

const (
	quicHTTPHeaderKey     = "HttpHeader"
	quicHTTPStatusKey     = "HttpStatus"
	quicMetadataFlowIDKey = "FlowID"
)

type RuntimeQUICConnection struct {
	conn                 quic.Connection
	logger               *zerolog.Logger
	orchestrator         *UpstreamOrchestrator
	datagramHandler      noOpSessionManager
	controlStreamHandler ControlStreamHandler
	connOptions          *runtimeConnectionOptionsSnapshot
	connIndex            uint8
	rpcTimeout           time.Duration
	streamWriteTimeout   time.Duration
	gracePeriod          time.Duration
}

type noOpSessionManager interface {
	Serve(context.Context) error
	tunnelpogs.SessionManager
}

func NewRuntimeQUICConnection(
	conn quic.Connection,
	orchestrator *UpstreamOrchestrator,
	datagramHandler noOpSessionManager,
	controlStreamHandler ControlStreamHandler,
	connOptions *runtimeConnectionOptionsSnapshot,
	connIndex uint8,
	rpcTimeout time.Duration,
	streamWriteTimeout time.Duration,
	gracePeriod time.Duration,
	logger *zerolog.Logger,
) *RuntimeQUICConnection {
	return &RuntimeQUICConnection{
		conn:                 conn,
		logger:               logger,
		orchestrator:         orchestrator,
		datagramHandler:      datagramHandler,
		controlStreamHandler: controlStreamHandler,
		connOptions:          connOptions,
		connIndex:            connIndex,
		rpcTimeout:           rpcTimeout,
		streamWriteTimeout:   streamWriteTimeout,
		gracePeriod:          gracePeriod,
	}
}

func (q *RuntimeQUICConnection) Serve(ctx context.Context) error {
	controlStream, err := q.conn.OpenStream()
	if err != nil {
		return fmt.Errorf("failed to open a registration control stream: %w", err)
	}

	errGroup, ctx := errgroup.WithContext(ctx)
	defer q.Close()

	errGroup.Go(func() error {
		if err := q.controlStreamHandler.ServeControlStream(ctx, controlStream, q.connOptions.ConnectionOptions(), q.orchestrator); err == nil {
			if q.gracePeriod > 0 {
				ticker := time.NewTicker(q.gracePeriod)
				defer ticker.Stop()
				select {
				case <-ctx.Done():
				case <-ticker.C:
				}
			}
		}
		return &ControlStreamError{}
	})
	errGroup.Go(func() error {
		err := q.acceptStream(ctx)
		if err != nil {
			q.logger.Error().Err(err).Msg("failed to accept incoming stream requests")
		}
		return &StreamListenerError{}
	})
	errGroup.Go(func() error {
		err := q.datagramHandler.Serve(ctx)
		if err != nil {
			q.logger.Error().Err(err).Msg("failed to run the datagram handler")
		}
		return &DatagramManagerError{}
	})
	return errGroup.Wait()
}

func (q *RuntimeQUICConnection) Close() {
	_ = q.conn.CloseWithError(0, "")
}

func (q *RuntimeQUICConnection) acceptStream(ctx context.Context) error {
	for {
		quicStream, err := q.conn.AcceptStream(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || q.controlStreamHandler.IsStopped() {
				return nil
			}
			return fmt.Errorf("failed to accept QUIC stream: %w", err)
		}
		go q.runStream(quicStream)
	}
}

func (q *RuntimeQUICConnection) runStream(quicStream quic.Stream) {
	ctx := quicStream.Context()
	stream := newRuntimeSafeStreamCloser(quicStream, q.streamWriteTimeout, q.logger)
	defer stream.Close()
	noCloseStream := &nopCloserReadWriter{ReadWriteCloser: stream}
	ss := newRuntimeCloudflaredServer(q.handleDataStream, q.datagramHandler, q, q.rpcTimeout)
	if err := ss.Serve(ctx, noCloseStream); err != nil {
		quicStream.CancelWrite(0)
	}
}

func (q *RuntimeQUICConnection) handleDataStream(ctx context.Context, stream *runtimeRequestServerStream) error {
	request, err := stream.ReadConnectRequestData()
	if err != nil {
		return err
	}
	if err, connectResponseSent := q.dispatchRequest(ctx, stream, request); err != nil {
		if connectResponseSent {
			return err
		}
		var metadata []runtimeMetadata
		if errors.Is(err, errTooManyActiveFlows) {
			metadata = append(metadata, runtimeErrorFlowConnectRateLimitedMetadata)
		}
		if writeRespErr := stream.WriteConnectResponseData(err, metadata...); writeRespErr != nil {
			return writeRespErr
		}
	}
	return nil
}

func (q *RuntimeQUICConnection) dispatchRequest(ctx context.Context, stream *runtimeRequestServerStream, request *runtimeConnectRequest) (err error, connectResponseSent bool) {
	originProxy, err := q.orchestrator.GetOriginProxy()
	if err != nil {
		return err, false
	}

	switch request.Type {
	case runtimeConnectionTypeHTTP, runtimeConnectionTypeWebsocket:
		tracedReq, err := buildRuntimeHTTPRequest(ctx, request, stream)
		if err != nil {
			return err, false
		}
		w := newRuntimeHTTPResponseAdapter(stream)
		return originProxy.ProxyHTTP(&w, tracedReq, request.Type == runtimeConnectionTypeWebsocket), w.connectResponseSent
	case runtimeConnectionTypeTCP:
		rwa := &runtimeStreamReadWriteAcker{runtimeRequestServerStream: stream}
		metadata := request.MetadataMap()
		return originProxy.ProxyTCP(ctx, rwa, &TCPRequest{
			Dest:      request.Dest,
			FlowID:    metadata[quicMetadataFlowIDKey],
			ConnIndex: q.connIndex,
		}), rwa.connectResponseSent
	default:
		return fmt.Errorf("unsupported error type: %s", request.Type), false
	}
}

func (q *RuntimeQUICConnection) UpdateConfiguration(ctx context.Context, version int32, config []byte) *runtimeUpdateConfigurationResponse {
	return q.orchestrator.UpdateConfig(version, config)
}

type runtimeStreamReadWriteAcker struct {
	*runtimeRequestServerStream
	connectResponseSent bool
}

func (s *runtimeStreamReadWriteAcker) AckConnection(string) error {
	s.connectResponseSent = true
	return s.WriteConnectResponseData(nil)
}

type runtimeHTTPResponseAdapter struct {
	*runtimeRequestServerStream
	headers             http.Header
	connectResponseSent bool
}

func newRuntimeHTTPResponseAdapter(s *runtimeRequestServerStream) runtimeHTTPResponseAdapter {
	return runtimeHTTPResponseAdapter{runtimeRequestServerStream: s, headers: make(http.Header)}
}

func (hrw *runtimeHTTPResponseAdapter) AddTrailer(string, string) {}

func (hrw *runtimeHTTPResponseAdapter) WriteRespHeaders(status int, header http.Header) error {
	metadata := make([]runtimeMetadata, 0)
	metadata = append(metadata, runtimeMetadata{Key: quicHTTPStatusKey, Val: strconv.Itoa(status)})
	for k, vv := range header {
		for _, v := range vv {
			httpHeaderKey := fmt.Sprintf("%s:%s", quicHTTPHeaderKey, k)
			metadata = append(metadata, runtimeMetadata{Key: httpHeaderKey, Val: v})
		}
	}
	return hrw.WriteConnectResponseData(nil, metadata...)
}

func (hrw *runtimeHTTPResponseAdapter) Write(p []byte) (int, error) {
	if !hrw.connectResponseSent {
		_ = hrw.WriteRespHeaders(http.StatusOK, hrw.headers)
	}
	return hrw.runtimeRequestServerStream.Write(p)
}

func (hrw *runtimeHTTPResponseAdapter) Header() http.Header { return hrw.headers }
func (hrw *runtimeHTTPResponseAdapter) Flush()              {}
func (hrw *runtimeHTTPResponseAdapter) WriteHeader(status int) {
	_ = hrw.WriteRespHeaders(status, hrw.headers)
}

func (hrw *runtimeHTTPResponseAdapter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn := &hijackedResponseConn{ReadWriteCloser: hrw.ReadWriteCloser}
	return conn, newHijackedReadWriter(hrw.ReadWriteCloser), nil
}

func (hrw *runtimeHTTPResponseAdapter) WriteConnectResponseData(respErr error, metadata ...runtimeMetadata) error {
	hrw.connectResponseSent = true
	return hrw.runtimeRequestServerStream.WriteConnectResponseData(respErr, metadata...)
}

func buildRuntimeHTTPRequest(ctx context.Context, request *runtimeConnectRequest, stream *runtimeRequestServerStream) (*TracedRequest, error) {
	req, err := http.NewRequestWithContext(ctx, request.MetadataMap()["HttpMethod"], request.Dest, stream)
	if err != nil {
		return nil, err
	}
	for _, metadata := range request.Metadata {
		if strings.HasPrefix(metadata.Key, quicHTTPHeaderKey+":") {
			req.Header.Add(strings.TrimPrefix(metadata.Key, quicHTTPHeaderKey+":"), metadata.Val)
		}
	}
	if req.Host == "" && req.URL != nil {
		req.Host = req.URL.Host
	}
	return NewTracedRequest(req), nil
}
