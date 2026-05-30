package health

import (
	"context"
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
}

type ReadyStatus struct {
	Ready   bool
	Summary string
}

type ReadyProvider func() ReadyStatus

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
	return ReadyStatus{Ready: true, Summary: "ready"}
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
