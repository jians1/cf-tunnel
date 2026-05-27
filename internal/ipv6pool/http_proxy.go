package ipv6pool

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

type HTTPProxy struct {
	listen string
	dialer ContextDialer
	logger *slog.Logger
}

func NewHTTPProxy(listen string, dialer ContextDialer, logger *slog.Logger) *HTTPProxy {
	return &HTTPProxy{
		listen: listen,
		dialer: dialer,
		logger: logger.With("proxy", "http"),
	}
}

func (p *HTTPProxy) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:    p.listen,
		Handler: p,
	}

	errCh := make(chan error, 1)
	go func() {
		p.logger.Info("http proxy listening", "listen", p.listen)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (p *HTTPProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleForward(w, r)
}

func (p *HTTPProxy) handleForward(w http.ResponseWriter, r *http.Request) {
	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	if outReq.URL.Scheme == "" {
		outReq.URL.Scheme = "http"
	}
	if outReq.URL.Host == "" {
		outReq.URL.Host = r.Host
	}
	removeHopByHopHeaders(outReq.Header)

	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           p.dialer.DialContext,
		ForceAttemptHTTP2:     false,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	resp, err := transport.RoundTrip(outReq)
	if err != nil {
		p.logger.Error("http proxy request failed", "method", r.Method, "host", outReq.URL.Host, "error", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	removeHopByHopHeaders(resp.Header)
	copyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *HTTPProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, buf, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	targetConn, err := p.dialer.DialContext(r.Context(), "tcp", canonicalAddress(r.Host, "443"))
	if err != nil {
		_ = clientConn.Close()
		p.logger.Error("http connect failed", "host", r.Host, "error", err)
		return
	}

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		_ = clientConn.Close()
		_ = targetConn.Close()
		return
	}

	if buf.Reader.Buffered() > 0 {
		if _, err := io.Copy(targetConn, buf.Reader); err != nil {
			_ = clientConn.Close()
			_ = targetConn.Close()
			return
		}
	}

	proxyBidirectional(r.Context(), clientConn, targetConn)
}

func proxyBidirectional(ctx context.Context, left, right net.Conn) {
	group, _ := errgroup.WithContext(ctx)
	group.Go(func() error {
		defer left.Close()
		defer right.Close()
		_, err := io.Copy(left, right)
		return err
	})
	group.Go(func() error {
		defer left.Close()
		defer right.Close()
		_, err := io.Copy(right, left)
		return err
	})
	_ = group.Wait()
}

func canonicalAddress(hostport, defaultPort string) string {
	if _, _, err := net.SplitHostPort(hostport); err == nil {
		return hostport
	}
	if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
		return net.JoinHostPort(strings.Trim(hostport, "[]"), defaultPort)
	}
	return net.JoinHostPort(hostport, defaultPort)
}

func removeHopByHopHeaders(header http.Header) {
	for _, key := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(key)
	}
	if connection := header.Get("Connection"); connection != "" {
		for _, field := range strings.Split(connection, ",") {
			header.Del(strings.TrimSpace(field))
		}
	}
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

var _ http.Handler = (*HTTPProxy)(nil)
var _ = bufio.ErrAdvanceTooFar
var _ = fmt.Sprintf
