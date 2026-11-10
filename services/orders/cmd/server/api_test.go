package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterOrdersEndpoints(t *testing.T) {
	orders := &stubOrders{}
	router := newRouter(orders)

	tests := []struct {
		name   string
		method string
		target string
		want   string
		status int
	}{
		{name: "list", method: http.MethodGet, target: "/api/orders", want: "GET /api/orders", status: http.StatusOK},
		{name: "detail", method: http.MethodGet, target: "/api/orders/11111111-1111-4111-8111-111111111111", want: "GET /api/orders/11111111-1111-4111-8111-111111111111", status: http.StatusOK},
		{name: "create", method: http.MethodPost, target: "/api/orders", want: "POST /api/orders", status: http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orders.called = ""
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))

			if rec.Code != tt.status {
				t.Fatalf("status = %d", rec.Code)
			}
			if orders.called != tt.want {
				t.Fatalf("called %q, want %q", orders.called, tt.want)
			}
		})
	}
}

func TestHealthDoesNotCallOrders(t *testing.T) {
	orders := &stubOrders{}
	router := newRouter(orders)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != "all good" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if orders.called != "" {
		t.Fatalf("orders handler called: %s", orders.called)
	}
}

type stubOrders struct {
	called string
}

func (s *stubOrders) GetOrders(w http.ResponseWriter, r *http.Request) {
	s.called = r.Method + " " + r.URL.Path
	w.WriteHeader(http.StatusOK)
}

func (s *stubOrders) GetOrder(w http.ResponseWriter, r *http.Request) {
	s.called = r.Method + " " + r.URL.Path
	w.WriteHeader(http.StatusOK)
}

func (s *stubOrders) CreateOrder(w http.ResponseWriter, r *http.Request) {
	s.called = r.Method + " " + r.URL.Path
	w.WriteHeader(http.StatusCreated)
}
