package runtime

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	cfdconnection "github.com/cloudflare/cloudflared/connection"
	cfdtracing "github.com/cloudflare/cloudflared/tracing"
)

type UpstreamOriginProxy struct {
	handler http.Handler
}

type websocketOriginProxy interface {
	ProxyWebsocket(cfdconnection.ResponseWriter, *http.Request) error
}

func NewUpstreamOriginProxy(handler http.Handler) *UpstreamOriginProxy {
	return &UpstreamOriginProxy{handler: handler}
}

func (p *UpstreamOriginProxy) ProxyHTTP(w cfdconnection.ResponseWriter, tr *cfdtracing.TracedHTTPRequest, isWebsocket bool) error {
	if p.handler == nil {
		return fmt.Errorf("nil origin handler")
	}

	restoreWebsocketUpgradeHeaders(tr, isWebsocket)
	if isWebsocket {
		if websocketProxy, ok := p.handler.(websocketOriginProxy); ok {
			return websocketProxy.ProxyWebsocket(w, tr.Request)
		}
	}

	recorder := newResponseWriterAdapter(w, isWebsocket)
	p.handler.ServeHTTP(recorder, tr.Request)
	return recorder.finalize()
}

func restoreWebsocketUpgradeHeaders(tr *cfdtracing.TracedHTTPRequest, isWebsocket bool) {
	if tr == nil || tr.Request == nil {
		return
	}
	if !isWebsocket && tr.Request.Header.Get(cfdconnection.InternalUpgradeHeader) != cfdconnection.WebsocketUpgrade {
		return
	}
	tr.Request.Header.Set("Connection", "Upgrade")
	tr.Request.Header.Set("Upgrade", "websocket")
	tr.Request.Header.Del(cfdconnection.InternalUpgradeHeader)
}

func (p *UpstreamOriginProxy) ProxyTCP(ctx context.Context, rwa cfdconnection.ReadWriteAcker, req *cfdconnection.TCPRequest) error {
	if req == nil {
		return fmt.Errorf("tcp request is nil")
	}
	if rwa == nil {
		return fmt.Errorf("tcp read writer is nil")
	}
	if req.Dest == "" {
		return fmt.Errorf("tcp destination is empty")
	}

	var dialer net.Dialer
	originConn, err := dialer.DialContext(ctx, "tcp", req.Dest)
	if err != nil {
		return fmt.Errorf("dial tcp origin %q: %w", req.Dest, err)
	}
	defer originConn.Close()

	if err := rwa.AckConnection(""); err != nil {
		return fmt.Errorf("ack tcp connection: %w", err)
	}

	errC := make(chan error, 2)
	go func() {
		_, err := io.Copy(originConn, rwa)
		errC <- err
	}()
	go func() {
		_, err := io.Copy(rwa, originConn)
		errC <- err
	}()

	select {
	case <-ctx.Done():
		_ = originConn.Close()
		return ctx.Err()
	case err := <-errC:
		_ = originConn.Close()
		if err != nil && !isBenignTCPProxyError(err) {
			return err
		}
		return nil
	}
}

func isBenignTCPProxyError(err error) bool {
	return err == nil || err == io.EOF || err == net.ErrClosed
}
