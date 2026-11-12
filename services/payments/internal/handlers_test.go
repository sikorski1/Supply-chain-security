package internal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestPaymentEndpoint(t *testing.T) {
	orderID := mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1")
	paymentID := mustUUID(t, "e0c5c1a3-45f0-4b0a-8b12-d44b0b4c3d18")
	payment := samplePayment(paymentID, orderID, 599800, statusCompleted)
	body := `{"orderId":"a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1","amountCents":599800}`

	t.Run("create", func(t *testing.T) {
		service := &fakeService{result: PaymentResult{Payment: payment, Created: true}}
		rec := serve(t, service, body)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if service.input.OrderID != orderID || service.input.AmountCents != 599800 {
			t.Fatalf("input = %+v", service.input)
		}
		var got map[string]any
		decodeResponse(t, rec, &got)
		if len(got) != 4 || got["id"] != paymentID.String() || got["orderId"] != orderID.String() || got["amountCents"] != float64(599800) || got["status"] != statusCompleted {
			t.Fatalf("payment = %+v", got)
		}
	})

	t.Run("failed payment", func(t *testing.T) {
		failed := payment
		failed.Status = statusFailed
		rec := serve(t, &fakeService{result: PaymentResult{Payment: failed, Created: true}}, body)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got paymentResponse
		decodeResponse(t, rec, &got)
		if got.Status != statusFailed || got.ID != paymentID.String() {
			t.Fatalf("payment = %+v", got)
		}
	})

	t.Run("existing payment returns ok", func(t *testing.T) {
		rec := serve(t, &fakeService{result: PaymentResult{Payment: payment, Created: false}}, body)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got paymentResponse
		decodeResponse(t, rec, &got)
		if got.ID != paymentID.String() || got.Status != statusCompleted {
			t.Fatalf("payment = %+v", got)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		service := &fakeService{}
		rec := serve(t, service, `{"orderId":`)
		assertAPIError(t, rec, http.StatusBadRequest, "invalid_request", "request body must be a JSON object")
		if service.called {
			t.Fatal("service was called")
		}
	})

	t.Run("invalid order id", func(t *testing.T) {
		service := &fakeService{}
		rec := serve(t, service, `{"orderId":"order-1","amountCents":100}`)
		assertAPIError(t, rec, http.StatusBadRequest, "invalid_request", "orderId must be a UUID")
		if service.called {
			t.Fatal("service was called")
		}
	})

	t.Run("validation error", func(t *testing.T) {
		rec := serve(t, &fakeService{err: &ValidationError{Message: "amountCents must be greater than 0"}}, `{"orderId":"a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1","amountCents":0}`)
		assertAPIError(t, rec, http.StatusBadRequest, "invalid_request", "amountCents must be greater than 0")
	})

	t.Run("different amount", func(t *testing.T) {
		rec := serve(t, &fakeService{err: ErrPaymentExists}, `{"orderId":"a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1","amountCents":100}`)
		assertAPIError(t, rec, http.StatusConflict, "payment_exists", "order already has a payment")
	})

	t.Run("database error stays generic", func(t *testing.T) {
		rec := serve(t, &fakeService{err: errors.New("duplicate key value violates unique constraint")}, body)
		assertAPIError(t, rec, http.StatusInternalServerError, "internal_error", "internal error")
	})
}

func serve(t *testing.T, service Service, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := chi.NewRouter()
	router.Post("/internal/payments", NewHandler(service).CreatePayment)

	req := httptest.NewRequest(http.MethodPost, "/internal/payments", strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, rec.Body.String())
	}
}

func assertAPIError(t *testing.T, rec *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got apiError
	decodeResponse(t, rec, &got)
	if got.Code != code || got.Message != message {
		t.Fatalf("error = %+v", got)
	}
}

type fakeService struct {
	result PaymentResult
	err    error
	input  CreatePaymentInput
	called bool
}

func (f *fakeService) CreatePayment(_ context.Context, input CreatePaymentInput) (PaymentResult, error) {
	f.called = true
	f.input = input
	if f.err != nil {
		return PaymentResult{}, f.err
	}
	return f.result, nil
}
