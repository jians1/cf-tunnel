package runtime

import (
	"net/http"

	"github.com/deanxv/cf-quicktunnel-ipv6pool/internal/cftunnel/protocol"
)

type TracedRequest = protocol.TracedRequest

func NewTracedRequest(req *http.Request) *TracedRequest {
	return &protocol.TracedRequest{Request: req}
}
