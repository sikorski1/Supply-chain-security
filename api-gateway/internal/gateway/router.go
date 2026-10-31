package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sikorski1/Supply-chain-security/services/api-gateway/internal/config"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewHandler(cfg config.Config) (http.Handler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	orders, err := NewProxy(cfg.OrdersServiceURL, cfg.UpstreamTimeout)
	if err != nil {
		return nil, fmt.Errorf("orders proxy: %w", err)
	}
	inventory, err := NewProxy(cfg.InventoryServiceURL, cfg.UpstreamTimeout)
	if err != nil {
		return nil, fmt.Errorf("inventory proxy: %w", err)
	}

	r := chi.NewRouter()

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		})
	})

	r.Get("/api/products", inventory.ServeHTTP)

	r.Get("/api/orders", orders.ServeHTTP)
	r.Get("/api/orders/{id}", orders.ServeHTTP)
	r.Post("/api/orders", orders.ServeHTTP)
	r.Get("/api/stats", orders.ServeHTTP)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, apiError{
			Code:    "not_found",
			Message: "route not found",
		})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusMethodNotAllowed, apiError{
			Code:    "method_not_allowed",
			Message: "method not allowed",
		})
	})

	return r, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
