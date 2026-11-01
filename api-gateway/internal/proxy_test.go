package internal

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestProxyForwardsRequestAndResponse(t *testing.T) {
	type captured struct {
		Method  string
		Path    string
		Query   string
		Body    string
		Headers map[string]string
	}

	var got captured
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		got = captured{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Body:   string(body),
			Headers: map[string]string{
				"Accept":            r.Header.Get("Accept"),
				"Authorization":     r.Header.Get("Authorization"),
				"Content-Type":      r.Header.Get("Content-Type"),
				"X-Request-ID":      r.Header.Get("X-Request-ID"),
				"X-Forwarded-For":   r.Header.Get("X-Forwarded-For"),
				"X-Forwarded-Host":  r.Header.Get("X-Forwarded-Host"),
				"X-Forwarded-Proto": r.Header.Get("X-Forwarded-Proto"),
				"Connection":        r.Header.Get("Connection"),
				"Keep-Alive":        r.Header.Get("Keep-Alive"),
				"X-Drop-Me":         r.Header.Get("X-Drop-Me"),
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream", "orders")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"order-1"}`))
	}))
	t.Cleanup(upstream.Close)

	proxy := newTestProxy(t, upstream.URL, time.Second)
	req := httptest.NewRequest(http.MethodPost, "http://gateway.test/api/orders?status=pending&status=open", strings.NewReader(`{"productId":"p1","quantity":2}`))
	req.Host = "gateway.test"
	req.RemoteAddr = "203.0.113.8:4242"
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-proxy")
	req.Header.Set("Connection", "close, X-Drop-Me")
	req.Header.Set("Keep-Alive", "timeout=5")
	req.Header.Set("X-Drop-Me", "secret")

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	want := captured{
		Method: http.MethodPost,
		Path:   "/api/orders",
		Query:  "status=pending&status=open",
		Body:   `{"productId":"p1","quantity":2}`,
		Headers: map[string]string{
			"Accept":            "application/json",
			"Authorization":     "Bearer token",
			"Content-Type":      "application/json",
			"X-Request-ID":      "req-proxy",
			"X-Forwarded-For":   "203.0.113.8",
			"X-Forwarded-Host":  "gateway.test",
			"X-Forwarded-Proto": "http",
			"Connection":        "",
			"Keep-Alive":        "",
			"X-Drop-Me":         "",
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("upstream request mismatch (-want +got):\n%s", diff)
	}

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.Equal(t, "orders", rec.Header().Get("X-Upstream"))
	require.JSONEq(t, `{"id":"order-1"}`, rec.Body.String())
}

func TestProxyUpstreamFailureModes(t *testing.T) {
	t.Run("connection error", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := listener.Addr().String()
		require.NoError(t, listener.Close())

		proxy := newTestProxy(t, "http://"+addr, time.Second)
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/orders", nil))

		require.Equal(t, http.StatusBadGateway, rec.Code)
		require.JSONEq(t, `{"code":"upstream_unavailable","message":"upstream request failed"}`, rec.Body.String())
	})

	t.Run("timeout", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(300 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(upstream.Close)

		proxy := newTestProxy(t, upstream.URL, 40*time.Millisecond)
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/stats", nil))

		require.Equal(t, http.StatusGatewayTimeout, rec.Code)
		require.JSONEq(t, `{"code":"upstream_timeout","message":"upstream request timed out"}`, rec.Body.String())
	})
}

func TestProxyForwardsUpstreamStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/api/orders/order-1")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(upstream.Close)

	proxy := newTestProxy(t, upstream.URL, time.Second)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/orders", nil))

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/api/orders/order-1", rec.Header().Get("Location"))
}

func TestNewProxyRejectsInvalidTarget(t *testing.T) {
	_, err := NewProxy("orders:8081", time.Second)
	require.Error(t, err)

	_, err = NewProxy("http://orders:8081", 0)
	require.Error(t, err)
}

func newTestProxy(t *testing.T, rawURL string, timeout time.Duration) http.Handler {
	t.Helper()
	proxy, err := NewProxy(rawURL, timeout)
	require.NoError(t, err)
	return proxy
}
