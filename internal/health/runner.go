package health

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
)

type Runner struct {
	listen string
	logger *slog.Logger
	ready  func() string
}

func NewRunner(listen string, logger *slog.Logger) *Runner {
	return &Runner{
		listen: listen,
		logger: logger.With("component", "health"),
	}
}

func (r *Runner) SetReadySummaryProvider(fn func() string) {
	r.ready = fn
}

func (r *Runner) ReadySummary() string {
	if r.ready != nil {
		return r.ready()
	}
	return "ready"
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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.ReadySummary()))
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
		return server.Shutdown(context.Background())
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
