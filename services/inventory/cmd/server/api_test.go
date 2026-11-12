package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterInventoryEndpoints(t *testing.T) {
	inventory := &stubInventory{}
	router := newRouter(inventory)

	tests := []struct {
		name   string
		method string
		target string
		want   string
		status int
	}{
		{name: "list", method: http.MethodGet, target: "/api/products", want: "GET /api/products", status: http.StatusOK},
		{name: "detail", method: http.MethodGet, target: "/internal/products/22222222-2222-4222-8222-222222222222", want: "GET /internal/products/22222222-2222-4222-8222-222222222222", status: http.StatusOK},
		{name: "reserve", method: http.MethodPost, target: "/internal/reservations", want: "POST /internal/reservations", status: http.StatusCreated},
		{name: "release", method: http.MethodPost, target: "/internal/reservations/11111111-1111-4111-8111-111111111111/release", want: "POST /internal/reservations/11111111-1111-4111-8111-111111111111/release", status: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inventory.called = ""
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))

			if rec.Code != tt.status {
				t.Fatalf("status = %d", rec.Code)
			}
			if inventory.called != tt.want {
				t.Fatalf("called %q, want %q", inventory.called, tt.want)
			}
		})
	}
}

func TestHealthDoesNotCallInventory(t *testing.T) {
	inventory := &stubInventory{}
	router := newRouter(inventory)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != "all good" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if inventory.called != "" {
		t.Fatalf("inventory handler called: %s", inventory.called)
	}
}

type stubInventory struct {
	called string
}

func (s *stubInventory) GetProducts(w http.ResponseWriter, r *http.Request) {
	s.called = r.Method + " " + r.URL.Path
	w.WriteHeader(http.StatusOK)
}

func (s *stubInventory) GetProduct(w http.ResponseWriter, r *http.Request) {
	s.called = r.Method + " " + r.URL.Path
	w.WriteHeader(http.StatusOK)
}

func (s *stubInventory) CreateReservation(w http.ResponseWriter, r *http.Request) {
	s.called = r.Method + " " + r.URL.Path
	w.WriteHeader(http.StatusCreated)
}

func (s *stubInventory) ReleaseReservation(w http.ResponseWriter, r *http.Request) {
	s.called = r.Method + " " + r.URL.Path
	w.WriteHeader(http.StatusOK)
}
