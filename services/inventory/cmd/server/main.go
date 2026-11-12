package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/sikorski1/Supply-chain-security/services/inventory/internal/env"
)

func main() {
	ctx := context.Background()

	cfg := config{
		addr: ":8082",
		db: dbConfig{
			dsn: env.GetString("GOOSE_DB_STRING", "host=localhost user=app password=app dbname=supply_chain sslmode=disable"),
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	conn, err := pgx.Connect(ctx, cfg.db.dsn)
	if err != nil {
		slog.Error("Server has failed to start", "error", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	logger.Info("Database connected successfully")

	api := application{
		config: cfg,
		db:     conn,
	}

	if err := api.run(api.mount()); err != nil {
		slog.Error("Server has failed to start", "error", err)
		os.Exit(1)
	}
}
