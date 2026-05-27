package runtime

import (
	"testing"
	"time"
)

func TestHTTP2ServerOptionsUsesDialConfigFactory(t *testing.T) {
	t.Parallel()

	opts := HTTP2ServerOptions{
		DialConfig: &HTTP2DialConfig{
			Address:   "example.com:443",
			Timeout:   2 * time.Second,
			TLSConfig: testTLSConfig(),
		},
	}

	opts = opts.withDefaults()
	if opts.TransportFactory == nil {
		t.Fatal("expected transport factory")
	}
	if _, ok := opts.TransportFactory.(DialHTTP2TransportFactory); !ok {
		t.Fatalf("expected dial transport factory, got %T", opts.TransportFactory)
	}
}

func TestHTTP2ServerOptionsFallsBackToPipeTransport(t *testing.T) {
	t.Parallel()

	opts := (HTTP2ServerOptions{}).withDefaults()
	if _, ok := opts.TransportFactory.(PipeHTTP2TransportFactory); !ok {
		t.Fatalf("expected pipe transport factory, got %T", opts.TransportFactory)
	}
}
