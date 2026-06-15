package origin

import (
	"io"
	"net/http"
	"testing"

	appconfig "github.com/jians1/cf-tunnel/internal/config"
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

func TestRoutedProxyStripsPathPrefix(t *testing.T) {
	t.Parallel()

	upstreamDefault := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("default"))
	}))
	defer upstreamDefault.Close()
	upstreamAPI := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.RequestURI()))
	}))
	defer upstreamAPI.Close()

	defaultTarget := Target{
		Raw:      upstreamDefault.URL,
		Protocol: ProtocolHTTP,
		URL:      MustParseURL(upstreamDefault.URL),
	}

	p, err := NewRoutedProxy(defaultTarget, []appconfig.RouteRule{
		{Path: "/api/*", Target: upstreamAPI.URL, StripPathPrefix: true},
	})
	if err != nil {
		t.Fatalf("new routed proxy: %v", err)
	}

	server := startLocalHTTPServer(t, p.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/ping?x=1")
	if err != nil {
		t.Fatalf("get api route: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "/ping?x=1" {
		t.Fatalf("unexpected stripped path: %q", string(body))
	}
}

func TestRoutedProxyDispatchByHostBeforePathFallback(t *testing.T) {
	t.Parallel()

	upstreamDefault := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("default"))
	}))
	defer upstreamDefault.Close()
	upstreamAPI := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("api"))
	}))
	defer upstreamAPI.Close()
	upstreamFallback := startLocalHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fallback"))
	}))
	defer upstreamFallback.Close()

	defaultTarget := Target{
		Raw:      upstreamDefault.URL,
		Protocol: ProtocolHTTP,
		URL:      MustParseURL(upstreamDefault.URL),
	}

	p, err := NewRoutedProxy(defaultTarget, []appconfig.RouteRule{
		{Host: "api.example.com", Path: "/api/*", Target: upstreamAPI.URL},
		{Path: "/api/*", Target: upstreamFallback.URL},
	})
	if err != nil {
		t.Fatalf("new routed proxy: %v", err)
	}

	server := startLocalHTTPServer(t, p.Handler())
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/ping", nil)
	if err != nil {
		t.Fatalf("build api request: %v", err)
	}
	req.Host = "api.example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get host route: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "api" {
		t.Fatalf("unexpected host route body: %q", string(body))
	}

	resp, err = http.Get(server.URL + "/api/ping")
	if err != nil {
		t.Fatalf("get fallback route: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "fallback" {
		t.Fatalf("unexpected fallback route body: %q", string(body))
	}
}

func TestRoutedProxyRoutesDoNotInheritDefaultTargetTLSOptions(t *testing.T) {
	t.Parallel()

	defaultTarget := Target{
		Raw:                "https://127.0.0.1:8443",
		Protocol:           ProtocolHTTPS,
		URL:                MustParseURL("https://127.0.0.1:8443"),
		ServerName:         "default.internal",
		InsecureSkipVerify: true,
	}

	p, err := NewRoutedProxy(defaultTarget, []appconfig.RouteRule{
		{Path: "/api/*", Target: "https://api.internal:9443"},
	})
	if err != nil {
		t.Fatalf("new routed proxy: %v", err)
	}

	proxy := p.proxyByTarget["host=,path=/api,target=https://api.internal:9443,server_name=,insecure_skip_verify=false"]
	if proxy == nil {
		t.Fatal("expected route proxy")
	}
	if proxy.target.ServerName != "" {
		t.Fatalf("route unexpectedly inherited server name: %s", proxy.target.ServerName)
	}
	if proxy.target.InsecureSkipVerify {
		t.Fatal("route unexpectedly inherited insecure skip verify")
	}
}

func TestRoutedProxyRoutesUseRouteTLSOptions(t *testing.T) {
	t.Parallel()

	defaultTarget := Target{
		Raw:      "https://default.internal:8443",
		Protocol: ProtocolHTTPS,
		URL:      MustParseURL("https://default.internal:8443"),
	}

	p, err := NewRoutedProxy(defaultTarget, []appconfig.RouteRule{
		{
			Path:               "/api/*",
			Target:             "https://127.0.0.1:9443",
			OriginServerName:   "api.internal",
			InsecureSkipVerify: true,
		},
	})
	if err != nil {
		t.Fatalf("new routed proxy: %v", err)
	}

	proxy := p.proxyByTarget["host=,path=/api,target=https://127.0.0.1:9443,server_name=api.internal,insecure_skip_verify=true"]
	if proxy == nil {
		t.Fatal("expected route proxy")
	}
	if proxy.target.ServerName != "api.internal" {
		t.Fatalf("unexpected route server name: %s", proxy.target.ServerName)
	}
	if !proxy.target.InsecureSkipVerify {
		t.Fatal("expected route insecure skip verify")
	}
}
