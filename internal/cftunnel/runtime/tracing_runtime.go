package runtime

import "net/http"

const (
	TracerContextName              = "cf-trace-id"
	IntCloudflaredTracingHeader    = "cf-int-cloudflared-tracing"
)

var CanonicalCloudflaredTracingHeader = http.CanonicalHeaderKey(IntCloudflaredTracingHeader)
