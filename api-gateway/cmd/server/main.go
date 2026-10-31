package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sikorski1/Supply-chain-security/services/api-gateway/internal/config"
	"github.com/sikorski1/Supply-chain-security/services/api-gateway/internal/gateway"
)

const (
	readHeaderTimeout   = 5 * time.Second
	readTimeout         = 30 * time.Second
	idleTimeout         = 90 * time.Second
	shutdownTimeout     = 10 * time.Second
	serverTimeoutMargin = 15 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "api-gateway")
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration is invalid", "error", err.Error())
		os.Exit(1)
	}

	handler, err := gateway.NewHandler(cfg)
	if err != nil {
		logger.Error("failed to create router", "error", err.Error())
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      cfg.UpstreamTimeout + serverTimeoutMargin,
		IdleTimeout:       idleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
	case err := <-errCh:
		logger.Error("server stopped", "error", err.Error())
		os.Exit(1)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err.Error())
		os.Exit(1)
	}
	logger.Info("stopped")
}
