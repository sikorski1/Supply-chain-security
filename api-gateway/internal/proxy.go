package internal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/sikorski1/Supply-chain-security/api-gateway/internal/json"
)

func NewProxy(rawURL string, timeout time.Duration) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(rawURL)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, fmt.Errorf("invalid upstream URL: %q", rawURL)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be positive")
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.SetXForwarded()
		},
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: timeout,
		},
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("upstream request failed", "error", err)

		status := http.StatusBadGateway
		code := "upstream_unavailable"
		message := "upstream request failed"

		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			status = http.StatusGatewayTimeout
			code = "upstream_timeout"
			message = "upstream request timed out"
		}

		json.Write(w, status, apiError{
			Code:    code,
			Message: message,
		})
	}

	return proxy, nil
}
