package tracing

import (
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewTracedHTTPRequestWrapsRequest(t *testing.T) {
	log := zerolog.Nop()
	req := httptest.NewRequest("GET", "http://localhost", nil)
	req.Header.Add(TracerContextName, "14cb070dde8e51fc5ae8514e69ba42ca:b38f1bf5eae406f3:0:1")

	tr := NewTracedHTTPRequest(req, 7, &log)

	if tr.Request != req {
		t.Fatal("expected traced request to wrap original request")
	}
	if tr.ConnIndex != 7 {
		t.Fatalf("expected conn index 7, got %d", tr.ConnIndex)
	}
	if got := tr.Header.Get(TracerContextName); got != "" {
		t.Fatalf("expected trace header to be stripped, got %q", got)
	}
}

func TestNoopSpanCanEnd(t *testing.T) {
	span := NewNoopSpan()
	End(span)
	EndWithErrorStatus(span, errMissingTracingID)
	EndWithStatusCode(span, 200)
}

func FuzzNewIdentity(f *testing.F) {
	f.Fuzz(func(t *testing.T, trace string) {
		_, _ = NewIdentity(trace)
	})
}
