package protocol

import (
	"context"
	"io"
	"net/http"
)

type ResponseWriter interface {
	WriteRespHeaders(status int, header http.Header) error
	AddTrailer(trailerName, trailerValue string)
	http.ResponseWriter
	http.Hijacker
	io.Writer
}

type ReadWriteAcker interface {
	io.ReadWriter
	AckConnection(tracePropagation string) error
}

type TCPRequest struct {
	Dest      string
	CFRay     string
	LBProbe   bool
	FlowID    string
	CfTraceID string
	ConnIndex uint8
}

type TracedRequest struct {
	Request *http.Request
}

type OriginProxy interface {
	ProxyHTTP(w ResponseWriter, tr *TracedRequest, isWebsocket bool) error
	ProxyTCP(ctx context.Context, rwa ReadWriteAcker, req *TCPRequest) error
}
