package origin

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	cfdconnection "github.com/cloudflare/cloudflared/connection"
)

const websocketCopyBufferSize = 32 * 1024

var websocketCopyBufferPool = sync.Pool{
	New: func() any {
		return make([]byte, websocketCopyBufferSize)
	},
}

type Proxy struct {
	target    Target
	transport *http.Transport
	proxy     *httputil.ReverseProxy
}

func NewProxy(target Target) *Proxy {
	transport := newTransport(target)
	director := func(req *http.Request) {
		rewriteRequest(req, target)
	}

	return &Proxy{
		target:    target,
		transport: transport,
		proxy: &httputil.ReverseProxy{
			Director:  director,
			Transport: transport,
		},
	}
}

func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(p.ServeHTTP)
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if p.target.WebsocketUpgradeMode && isWebsocketUpgrade(req) {
		p.proxyWebsocket(w, req)
		return
	}
	p.proxy.ServeHTTP(w, req)
}

func (p *Proxy) proxyWebsocket(w http.ResponseWriter, req *http.Request) {
	outReq := req.Clone(req.Context())
	outReq.Body = req.Body
	rewriteRequest(outReq, p.target)

	resp, err := p.transport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	backConn, ok := resp.Body.(io.ReadWriteCloser)
	if resp.StatusCode == http.StatusSwitchingProtocols && !ok {
		http.Error(w, "websocket origin response body is not writable", http.StatusBadGateway)
		return
	}

	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_, _ = copyWithWebsocketBuffer(w, resp.Body)
		return
	}

	clientConn, _, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	errC := make(chan error, 2)
	go copyAndClose(errC, backConn, clientConn)
	go copyAndClose(errC, clientConn, backConn)
	<-errC
}

func (p *Proxy) ProxyWebsocket(w cfdconnection.ResponseWriter, req *http.Request) error {
	outReq := req.Clone(req.Context())
	outReq.Body = nil
	outReq.ContentLength = 0
	rewriteRequest(outReq, p.target)

	resp, err := p.transport.RoundTrip(outReq)
	if err != nil {
		_ = w.WriteRespHeaders(http.StatusBadGateway, http.Header{})
		return fmt.Errorf("round trip websocket origin: %w", err)
	}
	defer resp.Body.Close()

	if err := w.WriteRespHeaders(resp.StatusCode, cloneHeader(resp.Header)); err != nil {
		return fmt.Errorf("write websocket response headers: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_, err := copyWithWebsocketBuffer(w, resp.Body)
		return err
	}

	backConn, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		return fmt.Errorf("websocket origin response body is not writable")
	}

	errC := make(chan error, 2)
	go copyAndClose(errC, backConn, req.Body)
	go func() {
		_, err := copyWithWebsocketBuffer(w, backConn)
		errC <- err
	}()
	return <-errC
}

func rewriteRequest(req *http.Request, target Target) {
	originalHost := req.Host
	req.URL.Scheme = target.URL.Scheme
	req.URL.Host = target.URL.Host
	req.Host = target.URL.Host

	if target.ServerName != "" {
		req.Host = target.ServerName
	}
	if originalHost != "" {
		req.Header.Set("X-Forwarded-Host", originalHost)
	}
}

func isWebsocketUpgrade(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Upgrade"), "websocket")
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func cloneHeader(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	copyHeader(dst, src)
	return dst
}

func copyAndClose(errC chan<- error, dst io.WriteCloser, src io.Reader) {
	_, err := copyWithWebsocketBuffer(dst, src)
	_ = dst.Close()
	errC <- err
}

func copyWithWebsocketBuffer(dst io.Writer, src io.Reader) (int64, error) {
	buf := getWebsocketCopyBuffer()
	n, err := io.CopyBuffer(dst, src, buf)
	putWebsocketCopyBuffer(buf)
	return n, err
}

func getWebsocketCopyBuffer() []byte {
	return websocketCopyBufferPool.Get().([]byte)
}

func putWebsocketCopyBuffer(buf []byte) {
	if len(buf) != websocketCopyBufferSize {
		return
	}
	websocketCopyBufferPool.Put(buf)
}

func newTransport(target Target) *http.Transport {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: target.InsecureSkipVerify, // nolint: gosec
	}
	if target.ServerName != "" {
		tlsConfig.ServerName = target.ServerName
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		MaxConnsPerHost:       256,
	}
}

func TargetURLString(target Target) string {
	if target.URL == nil {
		return ""
	}
	u := *target.URL
	return u.String()
}
