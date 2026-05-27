package runtime

import (
	"bufio"
	"fmt"
	"net"
	"net/http"

	cfdconnection "github.com/cloudflare/cloudflared/connection"
)

type responseWriterAdapter struct {
	upstream     cfdconnection.ResponseWriter
	header       http.Header
	status       int
	wroteHeader  bool
	sentHeader   bool
	websocket    bool
	hijackedConn net.Conn
}

func newResponseWriterAdapter(upstream cfdconnection.ResponseWriter, websocket bool) *responseWriterAdapter {
	return &responseWriterAdapter{
		upstream:  upstream,
		header:    make(http.Header),
		websocket: websocket,
	}
}

func (w *responseWriterAdapter) Header() http.Header {
	return w.header
}

func (w *responseWriterAdapter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.status = statusCode
	w.wroteHeader = true
}

func (w *responseWriterAdapter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if !w.sentHeader {
		if err := w.upstream.WriteRespHeaders(w.status, cloneHeader(w.header)); err != nil {
			return 0, err
		}
		w.sentHeader = true
	}
	return w.upstream.Write(p)
}

func (w *responseWriterAdapter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if !w.wroteHeader && w.websocket {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}
	if w.wroteHeader && !w.sentHeader {
		if err := w.upstream.WriteRespHeaders(w.status, cloneHeader(w.header)); err != nil {
			return nil, nil, err
		}
		w.sentHeader = true
	}
	conn, rw, err := w.upstream.Hijack()
	if err != nil {
		return nil, nil, err
	}
	w.hijackedConn = conn
	return conn, rw, nil
}

func (w *responseWriterAdapter) finalize() error {
	if w.hijackedConn != nil {
		return nil
	}
	if w.wroteHeader && !w.sentHeader {
		if err := w.upstream.WriteRespHeaders(w.status, cloneHeader(w.header)); err != nil {
			return err
		}
		w.sentHeader = true
	}
	return nil
}

func cloneHeader(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for k, values := range src {
		for _, value := range values {
			dst.Add(k, value)
		}
	}
	return dst
}

var _ http.ResponseWriter = (*responseWriterAdapter)(nil)
var _ http.Hijacker = (*responseWriterAdapter)(nil)

func (w *responseWriterAdapter) String() string {
	return fmt.Sprintf("responseWriterAdapter{status=%d}", w.status)
}
