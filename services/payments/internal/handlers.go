package internal

import (
	stdjson "encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/payments/internal/adapters/postgresql/sqlc"
	"github.com/sikorski1/Supply-chain-security/services/payments/internal/json"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type paymentResponse struct {
	ID          string `json:"id"`
	OrderID     string `json:"orderId"`
	AmountCents int32  `json:"amountCents"`
	Status      string `json:"status"`
}

type createPaymentRequest struct {
	OrderID     string `json:"orderId"`
	AmountCents int32  `json:"amountCents"`
}

type handler struct {
	service Service
}

func NewHandler(service Service) *handler {
	return &handler{service: service}
}

func (h *handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	var request createPaymentRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be a JSON object")
		return
	}

	orderID, err := parseUUID(request.OrderID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "orderId must be a UUID")
		return
	}

	result, err := h.service.CreatePayment(r.Context(), CreatePaymentInput{
		OrderID:     orderID,
		AmountCents: request.AmountCents,
	})
	if err != nil {
		h.writeServiceError(w, err)
		return
	}

	response, err := newPaymentResponse(result.Payment)
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

func (h *handler) writeServiceError(w http.ResponseWriter, err error) {
	var validation *ValidationError
	if errors.As(err, &validation) {
		writeError(w, http.StatusBadRequest, "invalid_request", validation.Error())
		return
	}
	if errors.Is(err, ErrPaymentExists) {
		writeError(w, http.StatusConflict, "payment_exists", "order already has a payment")
		return
	}
	h.writeInternalError(w, err)
}

func (h *handler) writeInternalError(w http.ResponseWriter, err error) {
	slog.Error("payment request failed", "error", err)
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

func newPaymentResponse(payment repository.Payment) (paymentResponse, error) {
	if !payment.ID.Valid || !payment.OrderID.Valid {
		return paymentResponse{}, errors.New("payment record is incomplete")
	}
	return paymentResponse{
		ID:          payment.ID.String(),
		OrderID:     payment.OrderID.String(),
		AmountCents: payment.AmountCents,
		Status:      payment.Status,
	}, nil
}
