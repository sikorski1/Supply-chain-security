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
	repository "github.com/sikorski1/Supply-chain-security/services/inventory/internal/adapters/postgresql/sqlc"
	"github.com/sikorski1/Supply-chain-security/services/inventory/internal/json"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type productResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PriceCents    int32  `json:"priceCents"`
	StockQuantity int32  `json:"stockQuantity"`
}

type reservationResponse struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"orderId"`
	ProductID string    `json:"productId"`
	Quantity  int32     `json:"quantity"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type createReservationRequest struct {
	OrderID   string `json:"orderId"`
	ProductID string `json:"productId"`
	Quantity  int32  `json:"quantity"`
}

type handler struct {
	service Service
}

func NewHandler(service Service) *handler {
	return &handler{service: service}
}

func (h *handler) GetProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.service.GetProducts(r.Context())
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	response := make([]productResponse, 0, len(products))
	for _, product := range products {
		item, err := newProductResponse(product)
		if err != nil {
			h.writeInternalError(w, err)
			return
		}
		response = append(response, item)
	}
	json.Write(w, http.StatusOK, response)
}

func (h *handler) GetProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "id must be a UUID")
		return
	}

	product, err := h.service.GetProduct(r.Context(), id)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	response, err := newProductResponse(product)
	if err != nil {
		h.writeInternalError(w, err)
		return
	}
	json.Write(w, http.StatusOK, response)
}

func (h *handler) CreateReservation(w http.ResponseWriter, r *http.Request) {
	var request createReservationRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be a JSON object")
		return
	}

	orderID, err := parseUUID(request.OrderID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "orderId must be a UUID")
		return
	}
	productID, err := parseUUID(request.ProductID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "productId must be a UUID")
		return
	}

	result, err := h.service.CreateReservation(r.Context(), CreateReservationInput{
		OrderID:   orderID,
		ProductID: productID,
		Quantity:  request.Quantity,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	response, err := newReservationResponse(result.Reservation)
	if err != nil {
		h.writeInternalError(w, err)
		return
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	json.Write(w, status, response)
}

func (h *handler) ReleaseReservation(w http.ResponseWriter, r *http.Request) {
	orderID, err := parseUUID(chi.URLParam(r, "orderId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "orderId must be a UUID")
		return
	}

	reservation, err := h.service.ReleaseReservation(r.Context(), orderID)
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	response, err := newReservationResponse(reservation)
	if err != nil {
		h.writeInternalError(w, err)
		return
	}
	json.Write(w, http.StatusOK, response)
}

func (h *handler) writeServiceError(w http.ResponseWriter, err error) {
	var validation *ValidationError
	if errors.As(err, &validation) {
		writeError(w, http.StatusBadRequest, "invalid_request", validation.Error())
		return
	}
	if errors.Is(err, ErrProductNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "product not found")
		return
	}
	if errors.Is(err, ErrReservationNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "reservation not found")
		return
	}
	if errors.Is(err, ErrInsufficientStock) {
		writeError(w, http.StatusConflict, "INSUFFICIENT_STOCK", "Not enough stock for this order.")
		return
	}
	if errors.Is(err, ErrReservationExists) {
		writeError(w, http.StatusConflict, "reservation_exists", "order already has an active reservation")
		return
	}
	h.writeInternalError(w, err)
}

func (h *handler) writeInternalError(w http.ResponseWriter, err error) {
	slog.Error("inventory request failed", "error", err)
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

func newProductResponse(product repository.Product) (productResponse, error) {
	if !product.ID.Valid {
		return productResponse{}, errors.New("product record is incomplete")
	}
	return productResponse{
		ID:            product.ID.String(),
		Name:          product.Name,
		PriceCents:    product.PriceCents,
		StockQuantity: product.StockQuantity,
	}, nil
}

func newReservationResponse(reservation repository.Reservation) (reservationResponse, error) {
	if !reservation.ID.Valid || !reservation.OrderID.Valid || !reservation.ProductID.Valid || !reservation.CreatedAt.Valid || !reservation.UpdatedAt.Valid {
		return reservationResponse{}, errors.New("reservation record is incomplete")
	}
	return reservationResponse{
		ID:        reservation.ID.String(),
		OrderID:   reservation.OrderID.String(),
		ProductID: reservation.ProductID.String(),
		Quantity:  reservation.Quantity,
		Status:    reservation.Status,
		CreatedAt: reservation.CreatedAt.Time.UTC(),
		UpdatedAt: reservation.UpdatedAt.Time.UTC(),
	}, nil
}
