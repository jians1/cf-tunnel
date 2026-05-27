package tracing

import "testing"

func TestNoopTracingExportsNoSpans(t *testing.T) {
	tracer := newNoopCfdTracer()

	if spans := tracer.GetSpans(); spans != "" {
		t.Fatalf("expected no encoded spans, got %q", spans)
	}
	if protoSpans := tracer.GetProtoSpans(); protoSpans != nil {
		t.Fatalf("expected no proto spans, got %d bytes", len(protoSpans))
	}
}
