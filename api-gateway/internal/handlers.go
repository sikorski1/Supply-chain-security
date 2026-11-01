package internal

import (
	"net/http"

	"github.com/sikorski1/Supply-chain-security/api-gateway/internal/json"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type handler struct {
	orders    http.Handler
	inventory http.Handler
}

func NewHandler(orders, inventory http.Handler) *handler {
	return &handler{
		orders:    orders,
		inventory: inventory,
	}
}

func (h *handler) Health(w http.ResponseWriter, r *http.Request) {
	json.Write(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func (h *handler) Products(w http.ResponseWriter, r *http.Request) {
	h.inventory.ServeHTTP(w, r)
}

func (h *handler) Orders(w http.ResponseWriter, r *http.Request) {
	h.orders.ServeHTTP(w, r)
}

func (h *handler) NotFound(w http.ResponseWriter, r *http.Request) {
	json.Write(w, http.StatusNotFound, apiError{
		Code:    "not_found",
		Message: "route not found",
	})
}

func (h *handler) MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	json.Write(w, http.StatusMethodNotAllowed, apiError{
		Code:    "method_not_allowed",
		Message: "method not allowed",
	})
}
