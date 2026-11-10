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
	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
)

func TestOrdersEndpoints(t *testing.T) {
	orderID := mustUUID(t, "11111111-1111-4111-8111-111111111111")
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	order := sampleOrder(orderID, productID)

	t.Run("list", func(t *testing.T) {
		rec := serve(t, &fakeService{orders: []repository.Order{order}}, http.MethodGet, "/api/orders", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got []orderResponse
		decodeResponse(t, rec, &got)
		if len(got) != 1 || got[0].ID != orderID.String() || got[0].ProductID != productID.String() {
			t.Fatalf("orders = %+v", got)
		}
		if got[0].ProductName != "Keyboard" || got[0].Quantity != 2 || got[0].UnitPriceCents != 7900 || got[0].Status != "pending" {
			t.Fatalf("order = %+v", got[0])
		}
		if !got[0].CreatedAt.Equal(order.CreatedAt.Time) || !got[0].UpdatedAt.Equal(order.UpdatedAt.Time) {
			t.Fatalf("timestamps = %s %s", got[0].CreatedAt, got[0].UpdatedAt)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		rec := serve(t, &fakeService{}, http.MethodGet, "/api/orders", "")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Fatalf("body = %s", rec.Body.String())
		}
	})

	t.Run("detail", func(t *testing.T) {
		service := &fakeService{order: order}
		rec := serve(t, service, http.MethodGet, "/api/orders/"+orderID.String(), "")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if service.gotID != orderID {
			t.Fatalf("lookup id = %s", service.gotID.String())
		}
		var got orderResponse
		decodeResponse(t, rec, &got)
		if got.ID != orderID.String() || got.ProductName != "Keyboard" {
			t.Fatalf("order = %+v", got)
		}
	})

	t.Run("missing order", func(t *testing.T) {
		rec := serve(t, &fakeService{err: ErrNotFound}, http.MethodGet, "/api/orders/"+orderID.String(), "")
		assertAPIError(t, rec, http.StatusNotFound, "not_found", "order not found")
	})

	t.Run("invalid id", func(t *testing.T) {
		rec := serve(t, &fakeService{}, http.MethodGet, "/api/orders/order-1", "")
		assertAPIError(t, rec, http.StatusBadRequest, "invalid_request", "id must be a UUID")
	})

	t.Run("create", func(t *testing.T) {
		service := &fakeService{order: order}
		body := `{"productId":"22222222-2222-4222-8222-222222222222","productName":"Keyboard","quantity":2,"unitPriceCents":7900}`
		rec := serve(t, service, http.MethodPost, "/api/orders", body)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if service.input.ProductID != productID || service.input.ProductName != "Keyboard" || service.input.Quantity != 2 || service.input.UnitPriceCents != 7900 {
			t.Fatalf("input = %+v", service.input)
		}
		var got orderResponse
		decodeResponse(t, rec, &got)
		if got.ID != orderID.String() || got.Status != "pending" {
			t.Fatalf("order = %+v", got)
		}
	})

	t.Run("invalid create body", func(t *testing.T) {
		rec := serve(t, &fakeService{}, http.MethodPost, "/api/orders", `{"productId":`)
		assertAPIError(t, rec, http.StatusBadRequest, "invalid_request", "request body must be a JSON object")
	})

	t.Run("invalid product id", func(t *testing.T) {
		service := &fakeService{}
		rec := serve(t, service, http.MethodPost, "/api/orders", `{"productId":"p1","productName":"Keyboard","quantity":1,"unitPriceCents":100}`)
		assertAPIError(t, rec, http.StatusBadRequest, "invalid_request", "productId must be a UUID")
		if service.called {
			t.Fatal("service was called")
		}
	})

	t.Run("validation error", func(t *testing.T) {
		rec := serve(t, &fakeService{err: &ValidationError{Message: "quantity must be greater than 0"}}, http.MethodPost, "/api/orders", `{"productId":"22222222-2222-4222-8222-222222222222","productName":"Keyboard","quantity":0,"unitPriceCents":100}`)
		assertAPIError(t, rec, http.StatusBadRequest, "invalid_request", "quantity must be greater than 0")
	})

	t.Run("database error stays generic", func(t *testing.T) {
		rec := serve(t, &fakeService{err: errors.New("duplicate key value violates unique constraint")}, http.MethodGet, "/api/orders", "")
		assertAPIError(t, rec, http.StatusInternalServerError, "internal_error", "internal error")
	})
}

func serve(t *testing.T, service Service, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewHandler(service)
	router := chi.NewRouter()
	router.Get("/api/orders", handler.GetOrders)
	router.Get("/api/orders/{id}", handler.GetOrder)
	router.Post("/api/orders", handler.CreateOrder)

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
	orders []repository.Order
	order  repository.Order
	err    error
	gotID  pgtype.UUID
	input  CreateOrderInput
	called bool
}

func (f *fakeService) GetOrders(context.Context) ([]repository.Order, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.orders, nil
}

func (f *fakeService) GetOrder(_ context.Context, id pgtype.UUID) (repository.Order, error) {
	f.gotID = id
	if f.err != nil {
		return repository.Order{}, f.err
	}
	return f.order, nil
}

func (f *fakeService) CreateOrder(_ context.Context, input CreateOrderInput) (repository.Order, error) {
	f.called = true
	f.input = input
	if f.err != nil {
		return repository.Order{}, f.err
	}
	return f.order, nil
}
