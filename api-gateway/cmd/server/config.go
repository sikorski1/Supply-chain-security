package main

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/sikorski1/Supply-chain-security/api-gateway/internal/env"
)

func loadConfig() (config, error) {
	port, err := strconv.Atoi(env.GetString("PORT", "8080"))
	if err != nil {
		return config{}, fmt.Errorf("PORT: %w", err)
	}
	if port < 1 || port > 65535 {
		return config{}, fmt.Errorf("PORT must be between 1 and 65535")
	}

	ordersServiceURL := env.GetString("ORDERS_SERVICE_URL", "http://orders:8081")
	if err := validateServiceURL("ORDERS_SERVICE_URL", ordersServiceURL); err != nil {
		return config{}, err
	}

	inventoryServiceURL := env.GetString("INVENTORY_SERVICE_URL", "http://inventory:8082")
	if err := validateServiceURL("INVENTORY_SERVICE_URL", inventoryServiceURL); err != nil {
		return config{}, err
	}

	upstreamTimeout, err := env.GetDuration("UPSTREAM_TIMEOUT", 15*time.Second)
	if err != nil {
		return config{}, err
	}
	if upstreamTimeout <= 0 {
		return config{}, fmt.Errorf("UPSTREAM_TIMEOUT must be greater than 0")
	}

	return config{
		addr:                fmt.Sprintf(":%d", port),
		ordersServiceURL:    ordersServiceURL,
		inventoryServiceURL: inventoryServiceURL,
		upstreamTimeout:     upstreamTimeout,
	}, nil
}

func validateServiceURL(name, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute http or https URL", name)
	}
	return nil
}
