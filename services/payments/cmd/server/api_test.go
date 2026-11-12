package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterPaymentEndpoint(t *testing.T) {
	payments := &stubPayments{}
	router := newRouter(payments)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/internal/payments", nil))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d", rec.Code)
	}
	if payments.called != "POST /internal/payments" {
		t.Fatalf("called %q", payments.called)
	}
}

func TestHealthDoesNotCallPayments(t *testing.T) {
	payments := &stubPayments{}
	router := newRouter(payments)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != "all good" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if payments.called != "" {
		t.Fatalf("payments handler called: %s", payments.called)
	}
}

type stubPayments struct {
	called string
}

func (s *stubPayments) CreatePayment(w http.ResponseWriter, r *http.Request) {
	s.called = r.Method + " " + r.URL.Path
	w.WriteHeader(http.StatusCreated)
}
