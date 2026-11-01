package main

import (
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	unsetEnv(t, "PORT", "ORDERS_SERVICE_URL", "INVENTORY_SERVICE_URL", "UPSTREAM_TIMEOUT")

	got, err := loadConfig()
	require.NoError(t, err)

	want := config{
		addr:                ":8080",
		ordersServiceURL:    "http://orders:8081",
		inventoryServiceURL: "http://inventory:8082",
		upstreamTimeout:     15 * time.Second,
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(config{})); diff != "" {
		t.Fatalf("loadConfig() mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadFromEnvironment(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("ORDERS_SERVICE_URL", "https://orders.internal:8443")
	t.Setenv("INVENTORY_SERVICE_URL", "http://127.0.0.1:8082")
	t.Setenv("UPSTREAM_TIMEOUT", "3s")

	got, err := loadConfig()
	require.NoError(t, err)

	want := config{
		addr:                ":9090",
		ordersServiceURL:    "https://orders.internal:8443",
		inventoryServiceURL: "http://127.0.0.1:8082",
		upstreamTimeout:     3 * time.Second,
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(config{})); diff != "" {
		t.Fatalf("loadConfig() mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		message string
	}{
		{
			name:    "port too low",
			env:     map[string]string{"PORT": "0"},
			message: "PORT",
		},
		{
			name:    "port too high",
			env:     map[string]string{"PORT": "65536"},
			message: "PORT",
		},
		{
			name:    "port not a number",
			env:     map[string]string{"PORT": "http"},
			message: "PORT",
		},
		{
			name:    "orders url missing scheme",
			env:     map[string]string{"ORDERS_SERVICE_URL": "orders:8081"},
			message: "ORDERS_SERVICE_URL",
		},
		{
			name:    "inventory url unsupported scheme",
			env:     map[string]string{"INVENTORY_SERVICE_URL": "ftp://inventory:8082"},
			message: "INVENTORY_SERVICE_URL",
		},
		{
			name:    "timeout not positive",
			env:     map[string]string{"UPSTREAM_TIMEOUT": "0s"},
			message: "UPSTREAM_TIMEOUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetEnv(t, "PORT", "ORDERS_SERVICE_URL", "INVENTORY_SERVICE_URL", "UPSTREAM_TIMEOUT")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			_, err := loadConfig()
			require.ErrorContains(t, err, tt.message)
		})
	}
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		t.Cleanup(func() {
			_ = os.Setenv(key, value)
		})
		require.NoError(t, os.Unsetenv(key))
	}
}
