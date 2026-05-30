package runtime

import (
	"net/http"

	"github.com/jians1/cf-tunnel/internal/cftunnel/protocol"
)

type TracedRequest = protocol.TracedRequest

func NewTracedRequest(req *http.Request) *TracedRequest {
	return &protocol.TracedRequest{Request: req}
}
