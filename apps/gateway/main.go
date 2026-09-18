package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chinmayarvind23/pulse_gate/apps/gateway/internal/config"
	"github.com/chinmayarvind23/pulse_gate/apps/gateway/internal/server"
)

// main wires configuration, transport and shutdown only. Business behavior stays in
// internal packages so it can be tested without binding a real socket. The service
// uses explicit shutdown because dropped in-flight webhook admissions can cause a
// provider retry and inflate duplicate pressure during deployments.
func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(2)
	}

	app, err := server.New(cfg)
	if err != nil {
		slog.Error("failed to initialize gateway", "error", err)
		os.Exit(2)
	}
	defer app.Close()

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       3 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("gateway listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway stopped unexpectedly", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	}
}
