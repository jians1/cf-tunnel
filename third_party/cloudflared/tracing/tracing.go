package tracing

import (
	"context"
	"net/http"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

const (
	TracerContextName = "cf-trace-id"

	IntCloudflaredTracingHeader = "cf-int-cloudflared-tracing"

	traceID128bitsWidth = 128 / 4
	separator           = ":"
)

var CanonicalCloudflaredTracingHeader = http.CanonicalHeaderKey(IntCloudflaredTracingHeader)

func Init(version string) {}

type TracedHTTPRequest struct {
	*http.Request
	*cfdTracer
	ConnIndex uint8
}

func NewTracedHTTPRequest(req *http.Request, connIndex uint8, log *zerolog.Logger) *TracedHTTPRequest {
	req.Header.Del(TracerContextName)
	return &TracedHTTPRequest{Request: req, cfdTracer: newNoopCfdTracer(), ConnIndex: connIndex}
}

func (tr *TracedHTTPRequest) ToTracedContext() *TracedContext {
	return &TracedContext{Context: tr.Context(), cfdTracer: tr.cfdTracer}
}

type TracedContext struct {
	context.Context
	*cfdTracer
}

func NewTracedContext(ctx context.Context, traceContext string, log *zerolog.Logger) *TracedContext {
	return &TracedContext{Context: ctx, cfdTracer: newNoopCfdTracer()}
}

type cfdTracer struct {
	trace.TracerProvider
}

func newNoopCfdTracer() *cfdTracer {
	return &cfdTracer{TracerProvider: trace.NewNoopTracerProvider()}
}

func (cft *cfdTracer) Tracer() trace.Tracer {
	return cft.TracerProvider.Tracer("origin")
}

func (cft *cfdTracer) GetSpans() string {
	return ""
}

func (cft *cfdTracer) GetProtoSpans() []byte {
	return nil
}

func (cft *cfdTracer) AddSpans(headers http.Header) {}

func End(span trace.Span) {
	if span != nil {
		span.End()
	}
}

func EndWithErrorStatus(span trace.Span, err error) {
	End(span)
}

func EndWithStatusCode(span trace.Span, statusCode int) {
	End(span)
}

func NewNoopSpan() trace.Span {
	_, span := trace.NewNoopTracerProvider().Tracer("origin").Start(context.Background(), "noop")
	return span
}
