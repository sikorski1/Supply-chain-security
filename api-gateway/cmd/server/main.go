package main

import (
	"log/slog"
	"os"

	"github.com/sikorski1/Supply-chain-security/api-gateway/internal"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("Server has failed to start", "error", err)
		os.Exit(1)
	}

	orders, err := internal.NewProxy(cfg.ordersServiceURL, cfg.upstreamTimeout)
	if err != nil {
		slog.Error("Server has failed to start", "error", err)
		os.Exit(1)
	}

	inventory, err := internal.NewProxy(cfg.inventoryServiceURL, cfg.upstreamTimeout)
	if err != nil {
		slog.Error("Server has failed to start", "error", err)
		os.Exit(1)
	}

	api := application{
		config:    cfg,
		orders:    orders,
		inventory: inventory,
	}

	if err := api.run(api.mount()); err != nil {
		slog.Error("Server has failed to start", "error", err)
		os.Exit(1)
	}
}
