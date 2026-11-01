package internal

import (
	"log"
	"net/http"

	"github.com/sikorski1/Supply-chain-security/services/orders/internal/json"
)

type handler struct {
	service Service
}

func NewHandler(service Service) *handler {
	return &handler{
		service: service,
	}
}

func (h *handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	// 1. CALL the service to get the orders
	// 2. Return JSON in a HTTP response

	orders, err := h.service.GetOrders(r.Context())
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	json.Write(w, http.StatusOK, orders)
}