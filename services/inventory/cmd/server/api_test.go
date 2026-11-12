package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	router := newRouter(nil)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != "all good" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}
