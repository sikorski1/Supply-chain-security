package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sikorski1/Supply-chain-security/api-gateway/internal"
	"github.com/stretchr/testify/require"
)

type upstreamEcho struct {
	Service string `json:"service"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Query   string `json:"query"`
	Body    string `json:"body"`
}

func TestRouterRoutesFrontendEndpoints(t *testing.T) {
	orders := newEchoServer(t, "orders")
	inventory := newEchoServer(t, "inventory")
	handler := newTestHandler(t, orders.URL, inventory.URL)

	tests := []struct {
		name   string
		method string
		target string
		body   string
		want   upstreamEcho
		status int
	}{
		{
			name:   "products go to inventory",
			method: http.MethodGet,
			target: "/api/products",
			want: upstreamEcho{
				Service: "inventory",
				Method:  http.MethodGet,
				Path:    "/api/products",
			},
		},
		{
			name:   "stats go to orders",
			method: http.MethodGet,
			target: "/api/stats",
			want: upstreamEcho{
				Service: "orders",
				Method:  http.MethodGet,
				Path:    "/api/stats",
			},
		},
		{
			name:   "order list goes to orders",
			method: http.MethodGet,
			target: "/api/orders?status=pending",
			want: upstreamEcho{
				Service: "orders",
				Method:  http.MethodGet,
				Path:    "/api/orders",
				Query:   "status=pending",
			},
		},
		{
			name:   "order detail goes to orders",
			method: http.MethodGet,
			target: "/api/orders/order-1",
			want: upstreamEcho{
				Service: "orders",
				Method:  http.MethodGet,
				Path:    "/api/orders/order-1",
			},
		},
		{
			name:   "create order goes to orders",
			method: http.MethodPost,
			target: "/api/orders",
			body:   `{"productId":"p1","quantity":2}`,
			want: upstreamEcho{
				Service: "orders",
				Method:  http.MethodPost,
				Path:    "/api/orders",
				Body:    `{"productId":"p1","quantity":2}`,
			},
			status: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			status := tt.status
			if status == 0 {
				status = http.StatusOK
			}
			require.Equal(t, status, rec.Code)

			var got upstreamEcho
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("upstream echo mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRouterHealthDoesNotCallUpstreams(t *testing.T) {
	handler := newTestHandler(t, "http://127.0.0.1:1", "http://127.0.0.1:1")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestRouterUnknownRouteAndMethod(t *testing.T) {
	orders := newEchoServer(t, "orders")
	inventory := newEchoServer(t, "inventory")
	handler := newTestHandler(t, orders.URL, inventory.URL)

	t.Run("unknown path", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/unknown", nil))

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.JSONEq(t, `{"code":"not_found","message":"route not found"}`, rec.Body.String())
	})

	t.Run("known path with the wrong method", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/orders", nil))

		require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		require.JSONEq(t, `{"code":"method_not_allowed","message":"method not allowed"}`, rec.Body.String())
	})
}

func TestRouterDownUpstreamWhileHealthStaysUp(t *testing.T) {
	handler := newTestHandler(t, "http://127.0.0.1:1", "http://127.0.0.1:1")

	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, httptest.NewRequest(http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusOK, healthRec.Code)

	orderRec := httptest.NewRecorder()
	handler.ServeHTTP(orderRec, httptest.NewRequest(http.MethodGet, "/api/orders", nil))
	require.Equal(t, http.StatusBadGateway, orderRec.Code)
	require.JSONEq(t, `{"code":"upstream_unavailable","message":"upstream request failed"}`, orderRec.Body.String())
}

func newTestHandler(t *testing.T, ordersURL, inventoryURL string) http.Handler {
	t.Helper()

	orders, err := internal.NewProxy(ordersURL, 2*time.Second)
	require.NoError(t, err)
	inventory, err := internal.NewProxy(inventoryURL, 2*time.Second)
	require.NoError(t, err)

	app := &application{
		config: config{
			addr:            ":8080",
			upstreamTimeout: 2 * time.Second,
		},
		orders:    orders,
		inventory: inventory,
	}
	return app.mount()
}

func newEchoServer(t *testing.T, service string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		status := http.StatusOK
		if r.Method == http.MethodPost {
			status = http.StatusCreated
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		require.NoError(t, json.NewEncoder(w).Encode(upstreamEcho{
			Service: service,
			Method:  r.Method,
			Path:    r.URL.Path,
			Query:   r.URL.RawQuery,
			Body:    string(body),
		}))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRouterDoesNotExposeInternalEndpoints(t *testing.T) {
	handler := newTestHandler(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	for _, path := range []string{"/internal/products/product-1", "/internal/reservations", "/internal/payments"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNotFound, rec.Code, path)
	}
}
