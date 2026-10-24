package ops

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeStatus(w, http.StatusOK, `{"status":"ok"}`)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		// M1-03/05 add storage, schema and verified active-artifact checks.
		// A live process must never imply a ready dictionary.
		writeStatus(w, http.StatusServiceUnavailable, `{"status":"not_ready","reason":"storage_not_initialized"}`)
	})
	return mux
}

func writeStatus(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body + "\n"))
}

// Run serves HTTP until cancellation, then drains requests within the configured deadline.
func Run(ctx context.Context, cfg Config, logger *slog.Logger) error {
	listener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		// Do not log arbitrary runtime input through net.OpError.
		return errors.New("HTTP listener unavailable")
	}
	server := &http.Server{
		Handler:           newHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	return serve(ctx, listener, server, cfg.ShutdownTimeout, logger)
}

func serve(ctx context.Context, listener net.Listener, server *http.Server, timeout time.Duration, logger *slog.Logger) error {
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	logger.Info("HTTP server started", "event", "http_started", "address", listener.Addr().String())
	select {
	case err := <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("HTTP server failed")
	case <-ctx.Done():
		logger.Info("HTTP server draining", "event", "http_draining")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			<-finished
			return errors.New("HTTP shutdown exceeded deadline")
		}
		<-finished
		logger.Info("HTTP server stopped", "event", "http_stopped")
		return nil
	}
}
