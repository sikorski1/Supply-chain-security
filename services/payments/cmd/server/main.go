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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sikorski1/Supply-chain-security/services/payments/internal/adapters/postgresql"
	"github.com/sikorski1/Supply-chain-security/services/payments/internal/env"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg := config{
		addr: env.GetString("ADDR", ":8083"),
		db:   dbConfig{dsn: env.GetString("GOOSE_DB_STRING", "postgresql://app:app@localhost:5432/supply_chain?sslmode=disable")},
	}
	pool, err := pgxpool.New(ctx, cfg.db.dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := pool.Ping(startup); err != nil {
		return err
	}
	if err := postgresql.Migrate(startup, pool); err != nil {
		return err
	}
	slog.Info("database connected and migrated")
	api := application{config: cfg, db: pool}
	if err := api.run(ctx, api.mount()); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
