package tracing

import (
	"context"
	"net/http"

	"github.com/rs/zerolog"
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
}

func newNoopCfdTracer() *cfdTracer {
	return &cfdTracer{}
}

type Attr struct{}

func StringAttr(_ string, _ string) Attr { return Attr{} }
func IntAttr(_ string, _ int) Attr       { return Attr{} }
func Int64Attr(_ string, _ int64) Attr   { return Attr{} }
func BoolAttr(_ string, _ bool) Attr     { return Attr{} }

type Span interface {
	End()
	SetAttributes(...Attr)
}

type Tracer interface {
	Start(context.Context, string, ...Attr) (context.Context, Span)
}

type noopSpan struct{}

func (noopSpan) End()                   {}
func (noopSpan) SetAttributes(...Attr)  {}

type noopTracer struct{}

func (noopTracer) Start(ctx context.Context, _ string, _ ...Attr) (context.Context, Span) {
	return ctx, noopSpan{}
}

func (cft *cfdTracer) Tracer() Tracer {
	return noopTracer{}
}

func (cft *cfdTracer) GetSpans() string {
	return ""
}

func (cft *cfdTracer) GetProtoSpans() []byte {
	return nil
}

func (cft *cfdTracer) AddSpans(headers http.Header) {}

func End(span Span) {
	if span != nil {
		span.End()
	}
}

func EndWithErrorStatus(span Span, err error) {
	End(span)
}

func EndWithStatusCode(span Span, statusCode int) {
	End(span)
}

func NewNoopSpan() Span {
	return noopSpan{}
}
