package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// handler is the HTTP handler for GET requests on any path. It validates the bearer token, scans resources, runs the
// worker pool, and streams JSON-lines results via chunked transfer encoding.
func handler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cfg.Token != "" && bearerToken(r.Header.Get("Authorization")) != cfg.Token {
			http.Error(w, "unauthorized: invalid bearer token", http.StatusUnauthorized)
			return
		}

		resources, err := ScanResources(cfg.ResourcesDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Scan error: %v\n", err)
			http.Error(w, fmt.Sprintf("scan error: %v", err), http.StatusInternalServerError)
			return
		}

		if len(resources) == 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		runChecks(cfg, resources, w, flusher.Flush)
	}
}

// runServer starts the HTTP server and blocks until SIGTERM/SIGINT. On signal it initiates graceful shutdown, waiting
// for in-flight requests to complete (up to the configured default timeout).
func runServer(cfg *Config) error {
	srv := &http.Server{
		Addr:    cfg.Listen,
		Handler: handler(cfg),
	}

	// Capture server startup errors on a channel that resolves quickly.
	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "SCOutpost server listening on %s\n", cfg.Listen)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for signal or startup error.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case sig := <-sigCh:
		fmt.Fprintf(os.Stderr, "Received %v, shutting down gracefully...\n", sig)
	}

	// Graceful shutdown with a deadline matching the default check timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.DefaultTimeout)*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}
	return nil
}
