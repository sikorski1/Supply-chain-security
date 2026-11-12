package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
)

type StatusStore interface {
	UpdateOrderStatus(context.Context, pgtype.UUID, string) (repository.Order, error)
}

type Publisher interface {
	Publish(context.Context, string, repository.Order) error
}

type UpstreamError struct {
	Status  int
	Code    string
	Message string
}

func (e *UpstreamError) Error() string { return e.Message }

type workflow struct {
	Service
	store     StatusStore
	inventory string
	payments  string
	publisher Publisher
	client    *http.Client
}

func NewWorkflow(base Service, store StatusStore, inventory, payments string, publisher Publisher) Service {
	return &workflow{Service: base, store: store, inventory: strings.TrimRight(inventory, "/"),
		payments: strings.TrimRight(payments, "/"), publisher: publisher, client: &http.Client{Timeout: 3 * time.Second}}
}

func (s *workflow) CreateOrder(ctx context.Context, input CreateOrderInput) (repository.Order, error) {
	// Leave time for detached compensation/reconciliation before Gateway's 25s timeout.
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !input.ProductID.Valid {
		return repository.Order{}, &ValidationError{Message: "productId must be a UUID"}
	}
	if input.Quantity < 1 {
		return repository.Order{}, &ValidationError{Message: "quantity must be greater than 0"}
	}
	var product struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Price int32  `json:"priceCents"`
	}
	if err := s.request(ctx, "GET", s.inventory+"/internal/products/"+input.ProductID.String(), nil, &product); err != nil {
		return repository.Order{}, err
	}
	// The Inventory snapshot is authoritative; retain the public request shape.
	input.ProductName, input.UnitPriceCents = product.Name, product.Price
	amount := int64(input.Quantity) * int64(product.Price)
	if amount > math.MaxInt32 {
		return repository.Order{}, &ValidationError{Message: "order amount is too large"}
	}
	order, err := s.Service.CreateOrder(ctx, input)
	if err != nil {
		return repository.Order{}, err
	}

	fail := func(cause error, release bool) (repository.Order, error) {
		// Cleanup must still run if the caller disconnected or its deadline expired.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		defer cancel()
		if release {
			releaseErr := s.request(cleanup, "POST", s.inventory+"/internal/reservations/"+order.ID.String()+"/release", struct{}{}, nil)
			var upstream *UpstreamError
			if releaseErr != nil && !(errors.As(releaseErr, &upstream) && upstream.Status == http.StatusNotFound) {
				return repository.Order{}, fmt.Errorf("order %s requires reservation reconciliation: %w", order.ID.String(), errors.Join(cause, releaseErr))
			}
		}
		failed, updateErr := s.store.UpdateOrderStatus(cleanup, order.ID, "failed")
		if updateErr != nil {
			return repository.Order{}, errors.Join(cause, updateErr)
		}
		if eventErr := s.publisher.Publish(cleanup, "orders.failed", failed); eventErr != nil {
			return repository.Order{}, errors.Join(cause, eventErr)
		}
		return failed, cause
	}
	if err := s.publisher.Publish(ctx, "orders.created", order); err != nil {
		return fail(err, false)
	}
	processing, err := s.store.UpdateOrderStatus(ctx, order.ID, "processing")
	if err != nil {
		return fail(err, false)
	}
	order = processing
	reservation := map[string]any{"orderId": order.ID.String(), "productId": input.ProductID.String(), "quantity": input.Quantity}
	if err := s.request(ctx, "POST", s.inventory+"/internal/reservations", reservation, nil); err != nil {
		var upstream *UpstreamError
		if errors.As(err, &upstream) && upstream.Status >= 400 && upstream.Status < 500 {
			return fail(err, false)
		}
		// Retry the same idempotent reservation after a lost/failed response.
		retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Second)
		err = s.request(retryCtx, "POST", s.inventory+"/internal/reservations", reservation, nil)
		cancel()
		if err != nil {
			// Releasing an invisible, still-committing reservation can return 404.
			// Preserve processing until its outcome is known instead of declaring failure.
			return repository.Order{}, &UpstreamError{Status: http.StatusBadGateway, Code: "reservation_unconfirmed", Message: fmt.Sprintf("reservation result for order %s is unconfirmed", order.ID.String())}
		}
	}

	// Inventory permits free products; Payments intentionally accepts positive amounts only.
	if amount > 0 {
		var payment struct {
			Status string `json:"status"`
		}
		body := map[string]any{"orderId": order.ID.String(), "amountCents": amount}
		err := s.request(ctx, "POST", s.payments+"/internal/payments", body, &payment)
		if err != nil {
			var upstream *UpstreamError
			if errors.As(err, &upstream) && upstream.Status >= 400 && upstream.Status < 500 {
				return fail(err, true)
			}
			// A lost payment response may hide a committed charge. The endpoint is
			// idempotent by order ID, so retry independently of the caller's context.
			retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Second)
			err = s.request(retryCtx, "POST", s.payments+"/internal/payments", body, &payment)
			cancel()
			if err != nil {
				// Keep processing + reserved until the payment result can be reconciled.
				return repository.Order{}, &UpstreamError{Status: http.StatusBadGateway, Code: "payment_unconfirmed", Message: fmt.Sprintf("payment result for order %s is unconfirmed", order.ID.String())}
			}
		}
		if payment.Status == "failed" {
			return fail(nil, true)
		}
		if payment.Status != "completed" {
			return fail(errors.New("unexpected payment status"), true)
		}
	}
	// Once payment has completed, keep the reservation even if persistence/publication fails.
	finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	order, err = s.store.UpdateOrderStatus(finalCtx, order.ID, "completed")
	if err != nil {
		return repository.Order{}, err
	}
	if err := s.publisher.Publish(finalCtx, "orders.completed", order); err != nil {
		return repository.Order{}, err
	}
	return order, nil
}

func (s *workflow) request(ctx context.Context, method, url string, body, result any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := s.client.Do(req)
	if err != nil {
		return &UpstreamError{Status: 502, Code: "upstream_unavailable", Message: "upstream request failed"}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var payload struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload)
		status := response.StatusCode
		if status >= 500 {
			status = http.StatusBadGateway
		}
		if payload.Code == "" {
			payload.Code = "upstream_error"
		}
		if payload.Message == "" {
			payload.Message = "upstream request failed"
		}
		return &UpstreamError{Status: status, Code: payload.Code, Message: payload.Message}
	}
	if result == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return err
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("invalid upstream response: %w", err)
	}
	return nil
}
