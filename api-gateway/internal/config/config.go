// Package config loads and validates the API gateway environment.
package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config is the process configuration for the gateway.
//
// The service URLs are the origins of the upstreams. Request paths are appended
// to them, so http://orders:8081 plus /api/orders becomes http://orders:8081/api/orders.
type Config struct {
	Port                int           `env:"PORT" envDefault:"8080"`
	OrdersServiceURL    string        `env:"ORDERS_SERVICE_URL" envDefault:"http://orders:8081"`
	InventoryServiceURL string        `env:"INVENTORY_SERVICE_URL" envDefault:"http://inventory:8082"`
	UpstreamTimeout     time.Duration `env:"UPSTREAM_TIMEOUT" envDefault:"15s"`
}

// Load reads configuration from the environment and validates it.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse environment: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks that the gateway can bind and reach both upstreams.
func (c Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("PORT must be between 1 and 65535")
	}
	if err := validateServiceURL("ORDERS_SERVICE_URL", c.OrdersServiceURL); err != nil {
		return err
	}
	if err := validateServiceURL("INVENTORY_SERVICE_URL", c.InventoryServiceURL); err != nil {
		return err
	}
	if c.UpstreamTimeout <= 0 {
		return fmt.Errorf("UPSTREAM_TIMEOUT must be greater than 0")
	}
	return nil
}

// Addr is the address passed to the HTTP server.
func (c Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}

func validateServiceURL(name, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute http or https URL", name)
	}
	return nil
}
