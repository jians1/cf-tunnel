package runtime

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

const proxyTCPBufferSize = 32 * 1024

var proxyTCPBufferPool = sync.Pool{
	New: func() any {
		return make([]byte, proxyTCPBufferSize)
	},
}

type UpstreamOriginProxy struct {
	handler http.Handler
}

type websocketOriginProxy interface {
	ProxyWebsocket(ResponseWriter, *http.Request) error
}

func NewUpstreamOriginProxy(handler http.Handler) *UpstreamOriginProxy {
	return &UpstreamOriginProxy{handler: handler}
}

func (p *UpstreamOriginProxy) ProxyHTTP(w ResponseWriter, tr *TracedRequest, isWebsocket bool) error {
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

func restoreWebsocketUpgradeHeaders(tr *TracedRequest, isWebsocket bool) {
	if tr == nil || tr.Request == nil {
		return
	}
	if !isWebsocket && tr.Request.Header.Get(InternalUpgradeHeader) != WebsocketUpgrade {
		return
	}
	tr.Request.Header.Set("Connection", "Upgrade")
	tr.Request.Header.Set("Upgrade", "websocket")
	tr.Request.Header.Del(InternalUpgradeHeader)
}

func (p *UpstreamOriginProxy) ProxyTCP(ctx context.Context, rwa ReadWriteAcker, req *TCPRequest) error {
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errC := make(chan error, 2)
	var copyWG sync.WaitGroup
	copyWG.Add(2)
	go func() {
		defer copyWG.Done()
		buf := getProxyTCPBuffer()
		defer putProxyTCPBuffer(buf)
		_, err := copyTCPStream(originConn, rwa, buf)
		errC <- err
	}()
	go func() {
		defer copyWG.Done()
		buf := getProxyTCPBuffer()
		defer putProxyTCPBuffer(buf)
		_, err := copyTCPStream(rwa, originConn, buf)
		errC <- err
	}()

	waitForCopies := func(initialErr error, pending int) error {
		firstErr := initialErr
		for range pending {
			if err := <-errC; firstErr == nil {
				firstErr = err
			}
		}
		copyWG.Wait()
		if firstErr != nil && !isBenignTCPProxyError(firstErr) {
			return firstErr
		}
		return nil
	}

	select {
	case <-ctx.Done():
		cancel()
		_ = originConn.Close()
		stopReadWriteAckerRead(rwa)
		_ = waitForCopies(nil, 2)
		return ctx.Err()
	case err := <-errC:
		cancel()
		_ = originConn.Close()
		stopReadWriteAckerRead(rwa)
		if tailErr := waitForCopies(err, 1); err == nil {
			err = tailErr
		}
		if err != nil && !isBenignTCPProxyError(err) {
			return err
		}
		return nil
	}
}

func stopReadWriteAckerRead(rwa ReadWriteAcker) {
	closer, ok := rwa.(interface{ CloseRead() error })
	if ok {
		_ = closer.CloseRead()
		return
	}
	if closer, ok := rwa.(io.Closer); ok {
		_ = closer.Close()
	}
}

func isBenignTCPProxyError(err error) bool {
	return err == nil || err == io.EOF || err == net.ErrClosed || strings.Contains(err.Error(), "use of closed network connection")
}

func getProxyTCPBuffer() []byte {
	buf, _ := proxyTCPBufferPool.Get().([]byte)
	if cap(buf) < proxyTCPBufferSize {
		return make([]byte, proxyTCPBufferSize)
	}
	return buf[:proxyTCPBufferSize]
}

func putProxyTCPBuffer(buf []byte) {
	if cap(buf) < proxyTCPBufferSize {
		return
	}
	proxyTCPBufferPool.Put(buf[:proxyTCPBufferSize])
}

func copyTCPStream(dst io.Writer, src io.Reader, buf []byte) (int64, error) {
	var written int64
	for {
		nr, readErr := src.Read(buf)
		if nr > 0 {
			nw, writeErr := dst.Write(buf[:nr])
			written += int64(nw)
			if writeErr != nil {
				return written, writeErr
			}
			if nw != nr {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return written, nil
			}
			return written, readErr
		}
	}
}
