package origin

import (
	"io"
	"net/http"
	"testing"

	appconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

func TestRoutedProxyDispatchByPath(t *testing.T) {
	t.Parallel()

	upstreamDefault := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("default"))
	}))
	defer upstreamDefault.Close()
	upstreamAPI := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("api"))
	}))
	defer upstreamAPI.Close()

	defaultTarget := Target{
		Raw:      upstreamDefault.URL,
		Protocol: ProtocolHTTP,
		URL:      MustParseURL(upstreamDefault.URL),
	}

	p, err := NewRoutedProxy(defaultTarget, []appconfig.RouteRule{
		{Path: "/api/*", Target: upstreamAPI.URL},
	})
	if err != nil {
		t.Fatalf("new routed proxy: %v", err)
	}

	server := startLocalHTTPServer(t, p.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/ping")
	if err != nil {
		t.Fatalf("get api route: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "api" {
		t.Fatalf("unexpected api route body: %q", string(body))
	}

	resp, err = http.Get(server.URL + "/other")
	if err != nil {
		t.Fatalf("get default route: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "default" {
		t.Fatalf("unexpected default route body: %q", string(body))
	}
}

