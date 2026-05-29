package origin

import (
	"testing"

	appconfig "github.com/deanxv/cf-quicktunnel-ipv6pool/internal/config"
)

func TestNewRouterBuildsAndMatchesWithPriority(t *testing.T) {
	t.Parallel()

	r, err := NewRouter([]appconfig.RouteRule{
		{Path: "/", Target: "http://127.0.0.1:9000"},
		{Path: "/api/*", Target: "http://127.0.0.1:9001"},
		{Path: "/api/admin/*", Target: "http://127.0.0.1:9002"},
		{Path: "/api/admin/health", Target: "http://127.0.0.1:9003"},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	tests := []struct {
		path       string
		wantTarget string
		wantOK     bool
	}{
		{path: "/api/admin/health", wantTarget: "http://127.0.0.1:9003", wantOK: true}, // exact
		{path: "/api/admin/users", wantTarget: "http://127.0.0.1:9002", wantOK: true},  // longest prefix
		{path: "/api/ping", wantTarget: "http://127.0.0.1:9001", wantOK: true},         // shorter prefix
		{path: "/other", wantTarget: "http://127.0.0.1:9000", wantOK: true},            // default
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			got, ok := r.Match("", tt.path)
			if ok != tt.wantOK {
				t.Fatalf("unexpected match status: got=%v want=%v", ok, tt.wantOK)
			}
			if got.Target != tt.wantTarget {
				t.Fatalf("unexpected target: got=%s want=%s", got.Target, tt.wantTarget)
			}
		})
	}
}

func TestRouterMatchNoRulesNoMatch(t *testing.T) {
	t.Parallel()

	r, err := NewRouter(nil)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	if _, ok := r.Match("", "/any"); ok {
		t.Fatal("expected no match without rules")
	}
}

func TestRouterMatchesHostSpecificRouteBeforePathFallback(t *testing.T) {
	t.Parallel()

	r, err := NewRouter([]appconfig.RouteRule{
		{Host: "api.example.com", Path: "/api/*", Target: "http://127.0.0.1:9001"},
		{Path: "/api/*", Target: "http://127.0.0.1:9002"},
	})
	if err != nil {
		t.Fatalf("new router: %v", err)
	}

	got, ok := r.Match("api.example.com", "/api/ping")
	if !ok {
		t.Fatal("expected host-specific match")
	}
	if got.Target != "http://127.0.0.1:9001" {
		t.Fatalf("unexpected host-specific target: %s", got.Target)
	}

	got, ok = r.Match("other.example.com", "/api/ping")
	if !ok {
		t.Fatal("expected path fallback match")
	}
	if got.Target != "http://127.0.0.1:9002" {
		t.Fatalf("unexpected fallback target: %s", got.Target)
	}
}

func TestNewRouterRejectsInvalidCompiledState(t *testing.T) {
	t.Parallel()

	_, err := NewRouter([]appconfig.RouteRule{
		{Path: "/api//*", Target: "http://127.0.0.1:9001"},
	})
	if err == nil {
		t.Fatal("expected build error for invalid route path")
	}
}
