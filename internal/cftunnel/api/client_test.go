package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCreateQuickTunnelSuccess(t *testing.T) {
	t.Parallel()

	server := startHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/tunnel" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"result":{"id":"11111111-1111-1111-1111-111111111111","name":"test","hostname":"demo.trycloudflare.com","account_tag":"acct","secret":"c2VjcmV0"}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-agent")
	reservation, err := client.CreateQuickTunnel(context.Background())
	if err != nil {
		t.Fatalf("create quick tunnel: %v", err)
	}
	if reservation.Hostname != "demo.trycloudflare.com" {
		t.Fatalf("unexpected hostname: %s", reservation.Hostname)
	}
	if reservation.URL != "https://demo.trycloudflare.com" {
		t.Fatalf("unexpected url: %s", reservation.URL)
	}
	if reservation.Credentials.AccountTag != "acct" {
		t.Fatalf("unexpected account tag: %s", reservation.Credentials.AccountTag)
	}
	if string(reservation.Credentials.TunnelSecret) != "secret" {
		t.Fatalf("unexpected tunnel secret: %q", string(reservation.Credentials.TunnelSecret))
	}
}

func TestCreateQuickTunnelFailure(t *testing.T) {
	t.Parallel()

	server := startHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1001,"message":"denied"}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-agent")
	if _, err := client.CreateQuickTunnel(context.Background()); err == nil {
		t.Fatal("expected error")
	} else if !strings.Contains(err.Error(), "1001:denied") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateQuickTunnelRejectsSuccessfulBodyWithHTTPErrorStatus(t *testing.T) {
	t.Parallel()

	server := startHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":true,"result":{"id":"11111111-1111-1111-1111-111111111111","name":"test","hostname":"demo.trycloudflare.com","account_tag":"acct","secret":"c2VjcmV0"}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-agent")
	if _, err := client.CreateQuickTunnel(context.Background()); err == nil {
		t.Fatal("expected error")
	} else if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status in error, got: %v", err)
	}
}

func TestCreateQuickTunnelRejectsIncompleteSuccessfulResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing id",
			body:    `{"success":true,"result":{"name":"test","hostname":"demo.trycloudflare.com","account_tag":"acct","secret":"c2VjcmV0"}}`,
			wantErr: "missing quick tunnel id",
		},
		{
			name:    "missing hostname",
			body:    `{"success":true,"result":{"id":"11111111-1111-1111-1111-111111111111","name":"test","account_tag":"acct","secret":"c2VjcmV0"}}`,
			wantErr: "missing quick tunnel hostname",
		},
		{
			name:    "missing account tag",
			body:    `{"success":true,"result":{"id":"11111111-1111-1111-1111-111111111111","name":"test","hostname":"demo.trycloudflare.com","secret":"c2VjcmV0"}}`,
			wantErr: "missing quick tunnel account tag",
		},
		{
			name:    "missing secret",
			body:    `{"success":true,"result":{"id":"11111111-1111-1111-1111-111111111111","name":"test","hostname":"demo.trycloudflare.com","account_tag":"acct"}}`,
			wantErr: "missing quick tunnel secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := startHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := NewClient(server.URL, "test-agent")
			if _, err := client.CreateQuickTunnel(context.Background()); err == nil {
				t.Fatal("expected error")
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCreateQuickTunnelNonJSONFailure(t *testing.T) {
	t.Parallel()

	server := startHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-agent")
	if _, err := client.CreateQuickTunnel(context.Background()); err == nil {
		t.Fatal("expected error")
	} else if !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateQuickTunnelRetriesRateLimitThenSucceeds(t *testing.T) {
	t.Parallel()

	var attempts int
	server := startHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("error code: 1015"))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"result":{"id":"11111111-1111-1111-1111-111111111111","name":"test","hostname":"demo.trycloudflare.com","account_tag":"acct","secret":"c2VjcmV0"}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-agent")
	reservation, err := client.CreateQuickTunnel(context.Background())
	if err != nil {
		t.Fatalf("create quick tunnel: %v", err)
	}
	if reservation.Hostname != "demo.trycloudflare.com" {
		t.Fatalf("unexpected hostname: %s", reservation.Hostname)
	}
	if attempts != 3 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
}

func TestCreateQuickTunnelUsesConfiguredRetryBackoff(t *testing.T) {
	t.Parallel()

	var attempts int
	server := startHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("error code: 1015"))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"result":{"id":"11111111-1111-1111-1111-111111111111","name":"test","hostname":"demo.trycloudflare.com","account_tag":"acct","secret":"c2VjcmV0"}}`))
	}))
	defer server.Close()

	client := NewClientWithOptions(server.URL, "test-agent", ClientOptions{
		RetryBackoffs: []time.Duration{time.Millisecond},
	})
	start := time.Now()
	if _, err := client.CreateQuickTunnel(context.Background()); err != nil {
		t.Fatalf("create quick tunnel: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("configured retry backoff was not used, elapsed %s", elapsed)
	}
	if attempts != 2 {
		t.Fatalf("unexpected attempts: %d", attempts)
	}
}

func TestIsRateLimitedError(t *testing.T) {
	t.Parallel()

	err := &QuickTunnelRateLimitedError{err: errors.New("rate limited")}
	if !IsRateLimitedError(err) {
		t.Fatal("expected rate limited error")
	}
	if IsRateLimitedError(errors.New("other")) {
		t.Fatal("did not expect non-rate-limited error")
	}
}

type testHTTPServer struct {
	ln     net.Listener
	server *http.Server
	URL    string
}

func (s *testHTTPServer) Close() {
	_ = s.server.Close()
	_ = s.ln.Close()
}

func startHTTPServer(t *testing.T, handler http.Handler) *testHTTPServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen http: %v", err)
	}

	srv := &http.Server{Handler: handler}
	go func() {
		_ = srv.Serve(ln)
	}()

	return &testHTTPServer{
		ln:     ln,
		server: srv,
		URL:    "http://" + ln.Addr().String(),
	}
}
