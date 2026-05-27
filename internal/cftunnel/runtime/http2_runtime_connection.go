package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"golang.org/x/net/http2"
)

var errRuntimeEdgeConnectionClosed = fmt.Errorf("connection with edge closed")

type RuntimeHTTP2Connection struct {
	conn                 net.Conn
	server               *http2.Server
	orchestrator         *UpstreamOrchestrator
	connOptions          *runtimeConnectionOptionsSnapshot
	connIndex            uint8
	log                  *zerolog.Logger
	activeRequestsWG     sync.WaitGroup
	controlStreamHandler ControlStreamHandler
	controlStreamErr     error
}

func NewRuntimeHTTP2Connection(
	conn net.Conn,
	orchestrator *UpstreamOrchestrator,
	connOptions *runtimeConnectionOptionsSnapshot,
	connIndex uint8,
	controlStreamHandler ControlStreamHandler,
	log *zerolog.Logger,
) *RuntimeHTTP2Connection {
	return &RuntimeHTTP2Connection{
		conn:                 conn,
		server:               &http2.Server{MaxConcurrentStreams: ^uint32(0)},
		orchestrator:         orchestrator,
		connOptions:          connOptions,
		connIndex:            connIndex,
		controlStreamHandler: controlStreamHandler,
		log:                  log,
	}
}

func (c *RuntimeHTTP2Connection) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		c.close()
	}()
	c.server.ServeConn(c.conn, &http2.ServeConnOpts{
		Context: ctx,
		Handler: c,
	})

	switch {
	case c.controlStreamHandler.IsStopped():
		return nil
	case c.controlStreamErr != nil:
		return c.controlStreamErr
	default:
		return errRuntimeEdgeConnectionClosed
	}
}

func (c *RuntimeHTTP2Connection) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.activeRequestsWG.Add(1)
	defer c.activeRequestsWG.Done()

	connType := determineHTTP2Type(r)
	handleMissingRequestParts(connType, r)

	respWriter, err := newRuntimeHTTP2RespWriter(r, w, connType, c.log)
	if err != nil {
		return
	}

	originProxy, err := c.orchestrator.GetOriginProxy()
	if err != nil {
		return
	}

	var requestErr error
	switch connType {
	case http2TypeControlStream:
		requestErr = c.controlStreamHandler.ServeControlStream(r.Context(), respWriter, c.connOptions.ConnectionOptions(), c.orchestrator)
		if requestErr != nil {
			c.controlStreamErr = requestErr
		}
	case http2TypeConfiguration:
		requestErr = c.handleConfigurationUpdate(respWriter, r)
	case http2TypeWebsocket, http2TypeHTTP:
		stripWebsocketUpgradeHeader(r)
		if err := originProxy.ProxyHTTP(respWriter, NewTracedRequest(r), connType == http2TypeWebsocket); err != nil {
			requestErr = fmt.Errorf("failed to proxy HTTP: %w", err)
		}
	case http2TypeTCP:
		host, err := getRequestHost(r)
		if err != nil {
			requestErr = fmt.Errorf("http2 request host missing: %w", err)
			break
		}
		rws := NewHTTPResponseReadWriterAcker(respWriter, respWriter, r)
		requestErr = originProxy.ProxyTCP(r.Context(), rws, &TCPRequest{
			Dest:      host,
			CFRay:     r.Header.Get("Cf-Ray"),
			ConnIndex: c.connIndex,
		})
	default:
		requestErr = fmt.Errorf("unknown http2 connection type: %s", connType)
	}

	if requestErr != nil {
		if !respWriter.WriteErrorResponse(requestErr) {
			panic(http.ErrAbortHandler)
		}
	}
}

func (c *RuntimeHTTP2Connection) handleConfigurationUpdate(respWriter *runtimeHTTP2RespWriter, r *http.Request) error {
	var body struct {
		Version int32           `json:"version"`
		Config  json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return err
	}
	resp := c.orchestrator.UpdateConfig(body.Version, body.Config)
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = respWriter.Write(b)
	return err
}

func (c *RuntimeHTTP2Connection) close() {
	c.activeRequestsWG.Wait()
	_ = c.conn.Close()
}

type http2RequestType string

const (
	http2TypeHTTP          http2RequestType = "http"
	http2TypeWebsocket     http2RequestType = "websocket"
	http2TypeTCP           http2RequestType = "tcp"
	http2TypeControlStream http2RequestType = "control-stream"
	http2TypeConfiguration http2RequestType = "configuration"
)

func (t http2RequestType) shouldFlush() bool {
	switch t {
	case http2TypeWebsocket, http2TypeTCP, http2TypeControlStream:
		return true
	default:
		return false
	}
}

type runtimeHTTP2RespWriter struct {
	r             io.Reader
	w             http.ResponseWriter
	flusher       http.Flusher
	shouldFlush   bool
	statusWritten bool
	respHeaders   http.Header
	hijackedMutex sync.Mutex
	hijackedv     bool
	log           *zerolog.Logger
}

func newRuntimeHTTP2RespWriter(r *http.Request, w http.ResponseWriter, connType http2RequestType, log *zerolog.Logger) (*runtimeHTTP2RespWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respWriter := &runtimeHTTP2RespWriter{r: r.Body, w: w, log: log}
		err := fmt.Errorf("%T doesn't implement http.Flusher", w)
		respWriter.WriteErrorResponse(err)
		return nil, err
	}
	return &runtimeHTTP2RespWriter{
		r:           r.Body,
		w:           w,
		flusher:     flusher,
		shouldFlush: connType.shouldFlush(),
		respHeaders: make(http.Header),
		log:         log,
	}, nil
}

func (rp *runtimeHTTP2RespWriter) AddTrailer(trailerName, trailerValue string) {
	if !rp.statusWritten {
		return
	}
	rp.w.Header().Add(http2.TrailerPrefix+trailerName, trailerValue)
}

func (rp *runtimeHTTP2RespWriter) WriteRespHeaders(status int, header http.Header) error {
	if rp.hijacked() {
		return nil
	}
	dest := rp.w.Header()
	userHeaders := make(http.Header, len(header))
	for name, values := range header {
		h2name := strings.ToLower(name)
		if h2name == "content-length" {
			dest[name] = values
		}
		if !isControlResponseHeader(h2name) || isWebsocketClientHeader(h2name) {
			userHeaders[name] = values
		}
	}
	dest.Set(CanonicalResponseUserHeaders, SerializeHeaders(userHeaders))
	dest.Set(CanonicalResponseMetaHeader, `{"src":"origin"}`)
	if status == http.StatusSwitchingProtocols {
		status = http.StatusOK
	}
	rp.w.WriteHeader(status)
	if shouldFlushResponse(header) {
		rp.shouldFlush = true
	}
	if rp.shouldFlush {
		rp.flusher.Flush()
	}
	rp.statusWritten = true
	return nil
}

func (rp *runtimeHTTP2RespWriter) Header() http.Header { return rp.respHeaders }
func (rp *runtimeHTTP2RespWriter) Flush()              { rp.flusher.Flush() }

func (rp *runtimeHTTP2RespWriter) WriteHeader(status int) {
	if rp.hijacked() {
		return
	}
	_ = rp.WriteRespHeaders(status, rp.respHeaders)
}

func (rp *runtimeHTTP2RespWriter) hijacked() bool {
	rp.hijackedMutex.Lock()
	defer rp.hijackedMutex.Unlock()
	return rp.hijackedv
}

func (rp *runtimeHTTP2RespWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if !rp.statusWritten {
		return nil, nil, fmt.Errorf("status not yet written before attempting to hijack connection")
	}
	if rp.shouldFlush {
		rp.flusher.Flush()
	}
	rp.hijackedMutex.Lock()
	defer rp.hijackedMutex.Unlock()
	if rp.hijackedv {
		return nil, nil, http.ErrHijacked
	}
	rp.hijackedv = true
	conn := &hijackedResponseConn{ReadWriteCloser: rp}
	return conn, newHijackedReadWriter(rp), nil
}

func (rp *runtimeHTTP2RespWriter) WriteErrorResponse(err error) bool {
	if rp.statusWritten {
		return false
	}
	if errors.Is(err, errTooManyActiveFlows) {
		rp.w.Header().Set(CanonicalResponseMetaHeader, `{"src":"cloudflared","flow_rate_limited":true}`)
	} else {
		rp.w.Header().Set(CanonicalResponseMetaHeader, `{"src":"cloudflared"}`)
	}
	rp.w.WriteHeader(http.StatusBadGateway)
	rp.statusWritten = true
	return true
}

func (rp *runtimeHTTP2RespWriter) Read(p []byte) (int, error) {
	return rp.r.Read(p)
}

func (rp *runtimeHTTP2RespWriter) Write(p []byte) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			_ = debug.Stack()
		}
	}()
	n, err = rp.w.Write(p)
	if err == nil && rp.shouldFlush {
		rp.flusher.Flush()
	}
	return n, err
}

func (rp *runtimeHTTP2RespWriter) Close() error { return nil }

func determineHTTP2Type(r *http.Request) http2RequestType {
	switch {
	case isConfigurationUpdate(r):
		return http2TypeConfiguration
	case isWebsocketUpgrade(r):
		return http2TypeWebsocket
	case isTCPStream(r):
		return http2TypeTCP
	case isControlStreamUpgrade(r):
		return http2TypeControlStream
	default:
		return http2TypeHTTP
	}
}

func handleMissingRequestParts(connType http2RequestType, r *http.Request) {
	if connType == http2TypeHTTP {
		if len(r.URL.Scheme) == 0 {
			r.URL.Scheme = "http"
		}
		if len(r.URL.Host) == 0 {
			r.URL.Host = "localhost:8080"
		}
	}
}

func isControlStreamUpgrade(r *http.Request) bool {
	return r.Header.Get(InternalUpgradeHeader) == ControlStreamUpgrade
}
func isWebsocketUpgrade(r *http.Request) bool {
	return r.Header.Get(InternalUpgradeHeader) == WebsocketUpgrade
}
func isConfigurationUpdate(r *http.Request) bool {
	return r.Header.Get(InternalUpgradeHeader) == ConfigurationUpdate
}
func isTCPStream(r *http.Request) bool { return r.Header.Get(InternalTCPProxySrcHeader) != "" }
func stripWebsocketUpgradeHeader(r *http.Request) {
	r.Header.Del(InternalUpgradeHeader)
}

func getRequestHost(r *http.Request) (string, error) {
	if r.Host != "" {
		return r.Host, nil
	}
	if r.URL != nil {
		return r.URL.Host, nil
	}
	return "", errors.New("host not set in incoming request")
}

func isControlResponseHeader(headerName string) bool {
	return strings.HasPrefix(headerName, ":") ||
		strings.HasPrefix(headerName, "cf-int-") ||
		strings.HasPrefix(headerName, "cf-cloudflared-") ||
		strings.HasPrefix(headerName, "cf-proxy-")
}

func isWebsocketClientHeader(headerName string) bool {
	return headerName == "sec-websocket-accept" || headerName == "connection" || headerName == "upgrade"
}

func shouldFlushResponse(headers http.Header) bool {
	if contentLength := headers.Get("content-length"); contentLength == "" {
		return true
	}
	if transferEncoding := headers.Get("transfer-encoding"); transferEncoding != "" {
		transferEncoding = strings.ToLower(transferEncoding)
		if strings.Contains(transferEncoding, "chunked") {
			return true
		}
	}
	if contentType := headers.Get("content-type"); contentType != "" {
		contentType = strings.ToLower(contentType)
		for _, c := range []string{"text/event-stream", "application/grpc", "application/x-ndjson"} {
			if strings.HasPrefix(contentType, c) {
				return true
			}
		}
	}
	return false
}
