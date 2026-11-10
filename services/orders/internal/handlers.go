package internal

import (
	stdjson "encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
	"github.com/sikorski1/Supply-chain-security/services/orders/internal/json"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type orderResponse struct {
	ID             string    `json:"id"`
	ProductID      string    `json:"productId"`
	ProductName    string    `json:"productName"`
	Quantity       int32     `json:"quantity"`
	UnitPriceCents int32     `json:"unitPriceCents"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type createOrderRequest struct {
	ProductID      string `json:"productId"`
	ProductName    string `json:"productName"`
	Quantity       int32  `json:"quantity"`
	UnitPriceCents int32  `json:"unitPriceCents"`
}

type handler struct {
	service Service
}

func NewHandler(service Service) *handler {
	return &handler{
		service: service,
	}
}

func (h *handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.service.GetOrders(r.Context())
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	response := make([]orderResponse, 0, len(orders))
	for _, order := range orders {
		item, err := newOrderResponse(order)
		if err != nil {
			h.writeInternalError(w, err)
			return
		}
		response = append(response, item)
	}

	json.Write(w, http.StatusOK, response)
}

func (h *handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "id must be a UUID")
		return
	}

	order, err := h.service.GetOrder(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	response, err := newOrderResponse(order)
	if err != nil {
		h.writeInternalError(w, err)
		return
	}

	json.Write(w, http.StatusOK, response)
}

func (h *handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var request createOrderRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be a JSON object")
		return
	}

	productID, err := parseUUID(request.ProductID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "productId must be a UUID")
		return
	}

	order, err := h.service.CreateOrder(r.Context(), CreateOrderInput{
		ProductID:      productID,
		ProductName:    request.ProductName,
		Quantity:       request.Quantity,
		UnitPriceCents: request.UnitPriceCents,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	response, err := newOrderResponse(order)
	if err != nil {
		h.writeInternalError(w, err)
		return
	}

	json.Write(w, http.StatusCreated, response)
}

func (h *handler) writeServiceError(w http.ResponseWriter, err error) {
	var validation *ValidationError
	if errors.As(err, &validation) {
		writeError(w, http.StatusBadRequest, "invalid_request", validation.Error())
		return
	}
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "order not found")
		return
	}
	h.writeInternalError(w, err)
}

func (h *handler) writeInternalError(w http.ResponseWriter, err error) {
	slog.Error("order request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	json.Write(w, status, apiError{
		Code:    code,
		Message: message,
	})
}

func decodeJSON(r *http.Request, dst any) error {
	decoder := stdjson.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func parseUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(strings.TrimSpace(value)); err != nil || !id.Valid {
		return pgtype.UUID{}, errors.New("invalid uuid")
	}
	return id, nil
}

func newOrderResponse(order repository.Order) (orderResponse, error) {
	if !order.ID.Valid || !order.ProductID.Valid || !order.CreatedAt.Valid || !order.UpdatedAt.Valid {
		return orderResponse{}, errors.New("order record is incomplete")
	}

	return orderResponse{
		ID:             order.ID.String(),
		ProductID:      order.ProductID.String(),
		ProductName:    order.ProductName,
		Quantity:       order.Quantity,
		UnitPriceCents: order.UnitPriceCents,
		Status:         order.Status,
		CreatedAt:      order.CreatedAt.Time.UTC(),
		UpdatedAt:      order.UpdatedAt.Time.UTC(),
	}, nil
}
