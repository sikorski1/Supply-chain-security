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
	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/inventory/internal/adapters/postgresql/sqlc"
)

func TestInventoryEndpoints(t *testing.T) {
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	orderID := mustUUID(t, "11111111-1111-4111-8111-111111111111")
	reservationID := mustUUID(t, "33333333-3333-4333-8333-333333333333")
	product := sampleProduct(productID, "Keyboard", 7900, 8)
	reservation := sampleReservation(reservationID, orderID, productID, 2, reservationActive)

	t.Run("list", func(t *testing.T) {
		rec := serve(t, &fakeService{products: []repository.Product{product}}, http.MethodGet, "/api/products", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got []productResponse
		decodeResponse(t, rec, &got)
		if len(got) != 1 || got[0] != (productResponse{ID: productID.String(), Name: "Keyboard", PriceCents: 7900, StockQuantity: 8}) {
			t.Fatalf("products = %+v", got)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		rec := serve(t, &fakeService{}, http.MethodGet, "/api/products", "")
		if strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Fatalf("body = %s", rec.Body.String())
		}
	})

	t.Run("product detail", func(t *testing.T) {
		service := &fakeService{product: product}
		rec := serve(t, service, http.MethodGet, "/internal/products/"+productID.String(), "")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if service.gotID != productID {
			t.Fatalf("lookup id = %s", service.gotID.String())
		}
	})

	t.Run("missing product", func(t *testing.T) {
		rec := serve(t, &fakeService{err: ErrProductNotFound}, http.MethodGet, "/internal/products/"+productID.String(), "")
		assertAPIError(t, rec, http.StatusNotFound, "not_found", "product not found")
	})

	t.Run("invalid product id", func(t *testing.T) {
		rec := serve(t, &fakeService{}, http.MethodGet, "/internal/products/p1", "")
		assertAPIError(t, rec, http.StatusBadRequest, "invalid_request", "id must be a UUID")
	})

	t.Run("reserve", func(t *testing.T) {
		service := &fakeService{reservation: ReservationResult{Reservation: reservation, Created: true}}
		body := `{"orderId":"11111111-1111-4111-8111-111111111111","productId":"22222222-2222-4222-8222-222222222222","quantity":2}`
		rec := serve(t, service, http.MethodPost, "/internal/reservations", body)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if service.input.OrderID != orderID || service.input.ProductID != productID || service.input.Quantity != 2 {
			t.Fatalf("input = %+v", service.input)
		}
		var got reservationResponse
		decodeResponse(t, rec, &got)
		if got.ID != reservationID.String() || got.Status != reservationActive || got.Quantity != 2 {
			t.Fatalf("reservation = %+v", got)
		}
	})

	t.Run("existing reservation returns ok", func(t *testing.T) {
		service := &fakeService{reservation: ReservationResult{Reservation: reservation, Created: false}}
		body := `{"orderId":"11111111-1111-4111-8111-111111111111","productId":"22222222-2222-4222-8222-222222222222","quantity":2}`
		rec := serve(t, service, http.MethodPost, "/internal/reservations", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("insufficient stock", func(t *testing.T) {
		body := `{"orderId":"11111111-1111-4111-8111-111111111111","productId":"22222222-2222-4222-8222-222222222222","quantity":9}`
		rec := serve(t, &fakeService{err: ErrInsufficientStock}, http.MethodPost, "/internal/reservations", body)
		assertAPIError(t, rec, http.StatusConflict, "INSUFFICIENT_STOCK", "Not enough stock for this order.")
	})

	t.Run("invalid reserve body", func(t *testing.T) {
		rec := serve(t, &fakeService{}, http.MethodPost, "/internal/reservations", `{"orderId":`)
		assertAPIError(t, rec, http.StatusBadRequest, "invalid_request", "request body must be a JSON object")
	})

	t.Run("release", func(t *testing.T) {
		released := reservation
		released.Status = reservationReleased
		service := &fakeService{released: released}
		rec := serve(t, service, http.MethodPost, "/internal/reservations/"+orderID.String()+"/release", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if service.orderID != orderID {
			t.Fatalf("order id = %s", service.orderID.String())
		}
		var got reservationResponse
		decodeResponse(t, rec, &got)
		if got.Status != reservationReleased {
			t.Fatalf("status = %s", got.Status)
		}
	})

	t.Run("missing reservation", func(t *testing.T) {
		rec := serve(t, &fakeService{err: ErrReservationNotFound}, http.MethodPost, "/internal/reservations/"+orderID.String()+"/release", "")
		assertAPIError(t, rec, http.StatusNotFound, "not_found", "reservation not found")
	})

	t.Run("database error stays generic", func(t *testing.T) {
		rec := serve(t, &fakeService{err: errors.New("connection refused")}, http.MethodGet, "/api/products", "")
		assertAPIError(t, rec, http.StatusInternalServerError, "internal_error", "internal error")
	})
}

func serve(t *testing.T, service Service, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewHandler(service)
	router := chi.NewRouter()
	router.Get("/api/products", handler.GetProducts)
	router.Get("/internal/products/{id}", handler.GetProduct)
	router.Post("/internal/reservations", handler.CreateReservation)
	router.Post("/internal/reservations/{orderId}/release", handler.ReleaseReservation)

	req := httptest.NewRequest(method, target, strings.NewReader(body))
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
	products    []repository.Product
	product     repository.Product
	reservation ReservationResult
	released    repository.Reservation
	err         error
	gotID       pgtype.UUID
	orderID     pgtype.UUID
	input       CreateReservationInput
}

func (f *fakeService) GetProducts(context.Context) ([]repository.Product, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.products, nil
}

func (f *fakeService) GetProduct(_ context.Context, id pgtype.UUID) (repository.Product, error) {
	f.gotID = id
	if f.err != nil {
		return repository.Product{}, f.err
	}
	return f.product, nil
}

func (f *fakeService) CreateReservation(_ context.Context, input CreateReservationInput) (ReservationResult, error) {
	f.input = input
	if f.err != nil {
		return ReservationResult{}, f.err
	}
	return f.reservation, nil
}

func (f *fakeService) ReleaseReservation(_ context.Context, orderID pgtype.UUID) (repository.Reservation, error) {
	f.orderID = orderID
	if f.err != nil {
		return repository.Reservation{}, f.err
	}
	return f.released, nil
}
