package runtime

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jians1/cf-tunnel/internal/cftunnel/protocol"
)

const (
	InternalUpgradeHeader     = "Cf-Cloudflared-Proxy-Connection-Upgrade"
	InternalTCPProxySrcHeader = "Cf-Cloudflared-Proxy-Src"
	WebsocketUpgrade          = "websocket"
	ControlStreamUpgrade      = "control-stream"
	ConfigurationUpdate       = "update-configuration"

	RequestUserHeaders  = "cf-cloudflared-request-headers"
	ResponseUserHeaders = "cf-cloudflared-response-headers"
	ResponseMetaHeader  = "cf-cloudflared-response-meta"
)

var (
	CanonicalResponseUserHeaders = http.CanonicalHeaderKey(ResponseUserHeaders)
	CanonicalResponseMetaHeader  = http.CanonicalHeaderKey(ResponseMetaHeader)
)

type HTTPHeader struct {
	Name  string
	Value string
}

type ResponseWriter = protocol.ResponseWriter
type ReadWriteAcker = protocol.ReadWriteAcker
type OriginProxy = protocol.OriginProxy
type TCPRequest = protocol.TCPRequest

type ConnectedFuse interface {
	Connected()
	IsConnected() bool
}

type TunnelConfigJSONGetter interface {
	GetConfigJSON() ([]byte, error)
}

type ControlStreamHandler interface {
	ServeControlStream(ctx context.Context, rw io.ReadWriteCloser, connOptions *runtimeConnectionOptions, tunnelConfigGetter TunnelConfigJSONGetter) error
	IsStopped() bool
}

var headerEncoding = base64.RawStdEncoding

func SerializeHeaders(h1Headers http.Header) string {
	serializedLen := 0
	maxTempLen := 0
	for headerName, headerValues := range h1Headers {
		for _, headerValue := range headerValues {
			nameLen := headerEncoding.EncodedLen(len(headerName))
			valueLen := headerEncoding.EncodedLen(len(headerValue))
			serializedLen += 2 + nameLen + valueLen
			if nameLen > maxTempLen {
				maxTempLen = nameLen
			}
			if valueLen > maxTempLen {
				maxTempLen = valueLen
			}
		}
	}
	var buf strings.Builder
	buf.Grow(serializedLen)
	temp := make([]byte, maxTempLen)
	writeB64 := func(s string) {
		n := headerEncoding.EncodedLen(len(s))
		if n > len(temp) {
			temp = make([]byte, n)
		}
		headerEncoding.Encode(temp[:n], []byte(s))
		buf.Write(temp[:n])
	}
	for headerName, headerValues := range h1Headers {
		for _, headerValue := range headerValues {
			if buf.Len() > 0 {
				buf.WriteByte(';')
			}
			writeB64(headerName)
			buf.WriteByte(':')
			writeB64(headerValue)
		}
	}
	return buf.String()
}

func DeserializeHeaders(serializedHeaders string) ([]HTTPHeader, error) {
	deserialized := make([]HTTPHeader, 0)
	for _, serializedPair := range strings.Split(serializedHeaders, ";") {
		if len(serializedPair) == 0 {
			continue
		}
		serializedHeaderParts := strings.Split(serializedPair, ":")
		if len(serializedHeaderParts) != 2 {
			return nil, fmt.Errorf("unable to deserialize headers")
		}
		serializedName := serializedHeaderParts[0]
		serializedValue := serializedHeaderParts[1]
		deserializedName := make([]byte, headerEncoding.DecodedLen(len(serializedName)))
		deserializedValue := make([]byte, headerEncoding.DecodedLen(len(serializedValue)))
		if _, err := headerEncoding.Decode(deserializedName, []byte(serializedName)); err != nil {
			return nil, fmt.Errorf("unable to deserialize headers: %w", err)
		}
		if _, err := headerEncoding.Decode(deserializedValue, []byte(serializedValue)); err != nil {
			return nil, fmt.Errorf("unable to deserialize headers: %w", err)
		}
		deserialized = append(deserialized, HTTPHeader{Name: string(deserializedName), Value: string(deserializedValue)})
	}
	return deserialized, nil
}

type hijackedResponseConn struct {
	io.ReadWriteCloser
}

func (c *hijackedResponseConn) LocalAddr() net.Addr                { return &net.TCPAddr{IP: net.IPv6loopback} }
func (c *hijackedResponseConn) RemoteAddr() net.Addr               { return &net.TCPAddr{IP: net.IPv6loopback} }
func (c *hijackedResponseConn) SetDeadline(_ time.Time) error      { return nil }
func (c *hijackedResponseConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *hijackedResponseConn) SetWriteDeadline(_ time.Time) error { return nil }

func newHijackedReadWriter(rw io.ReadWriteCloser) *bufio.ReadWriter {
	return bufio.NewReadWriter(bufio.NewReader(rw), bufio.NewWriter(rw))
}

type HTTPResponseReadWriteAcker struct {
	r   io.Reader
	w   ResponseWriter
	f   http.Flusher
	req *http.Request
}

func NewHTTPResponseReadWriterAcker(w ResponseWriter, flusher http.Flusher, req *http.Request) *HTTPResponseReadWriteAcker {
	return &HTTPResponseReadWriteAcker{r: req.Body, w: w, f: flusher, req: req}
}

func (h *HTTPResponseReadWriteAcker) Read(p []byte) (int, error) {
	return h.r.Read(p)
}

func (h *HTTPResponseReadWriteAcker) Write(p []byte) (int, error) {
	n, err := h.w.Write(p)
	if n > 0 {
		h.f.Flush()
	}
	return n, err
}

func (h *HTTPResponseReadWriteAcker) AckConnection(string) error {
	resp := &http.Response{
		StatusCode:    http.StatusSwitchingProtocols,
		ContentLength: -1,
		Header:        http.Header{},
	}
	if secWebsocketKey := h.req.Header.Get("Sec-WebSocket-Key"); secWebsocketKey != "" {
		resp.Header.Set("Connection", "Upgrade")
		resp.Header.Set("Upgrade", "websocket")
	}
	return h.w.WriteRespHeaders(resp.StatusCode, resp.Header)
}
