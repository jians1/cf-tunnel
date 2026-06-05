package health

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

const shutdownTimeout = 1 * time.Second

type Runner struct {
	listen string
	logger *slog.Logger
	ready  ReadyProvider
	status StatusProvider
}

type ReadyStatus struct {
	Ready   bool
	Summary string
}

type ReadyProvider func() ReadyStatus

type TunnelStatus struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	QuickTunnel    bool   `json:"quick_tunnel"`
	QuickTunnelURL string `json:"quick_tunnel_url"`
	Hostname       string `json:"hostname"`
	Protocol       string `json:"protocol"`
	OriginURL      string `json:"origin_url"`
}

type StatusPayload struct {
	Mode    string         `json:"mode"`
	Ready   bool           `json:"ready"`
	Summary string         `json:"summary"`
	Tunnel  *TunnelStatus  `json:"tunnel,omitempty"`
	Tunnels []TunnelStatus `json:"tunnels,omitempty"`
}

type StatusProvider func() StatusPayload

func NewRunner(listen string, logger *slog.Logger) *Runner {
	return &Runner{
		listen: listen,
		logger: logger.With("component", "health"),
	}
}

func (r *Runner) SetReadySummaryProvider(fn func() string) {
	r.ready = func() ReadyStatus {
		return ReadyStatus{Ready: true, Summary: fn()}
	}
}

func (r *Runner) SetReadyProvider(fn ReadyProvider) {
	r.ready = fn
}

func (r *Runner) SetStatusProvider(fn StatusProvider) {
	r.status = fn
}

func (r *Runner) ReadySummary() string {
	return r.ReadyStatus().Summary
}

func (r *Runner) ReadyStatus() ReadyStatus {
	if r.ready != nil {
		status := r.ready()
		if status.Summary == "" {
			status.Summary = "not ready"
		}
		return status
	}
	return ReadyStatus{Ready: false, Summary: "not ready"}
}

func (r *Runner) Status() StatusPayload {
	if r.status != nil {
		status := r.status()
		if status.Summary == "" {
			status.Summary = "not ready"
		}
		return status
	}
	return StatusPayload{
		Ready:   false,
		Summary: "not ready",
	}
}

func (r *Runner) Name() string {
	return "health"
}

func (r *Runner) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		status := r.ReadyStatus()
		if status.Ready {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_, _ = w.Write([]byte(status.Summary))
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(r.Status()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	server := &http.Server{
		Addr:    r.listen,
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		r.logger.Info("health server listening", "listen", r.listen)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
