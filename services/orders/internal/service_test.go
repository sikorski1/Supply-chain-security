package internal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
)

func TestGetOrdersReturnsEmptySlice(t *testing.T) {
	repo := &stubRepo{}
	service := NewService(repo)

	orders, err := service.GetOrders(context.Background())
	if err != nil {
		t.Fatalf("GetOrders: %v", err)
	}
	if orders == nil || len(orders) != 0 {
		t.Fatalf("orders = %#v, want empty slice", orders)
	}
}

func TestGetOrderNotFound(t *testing.T) {
	repo := &stubRepo{findErr: pgx.ErrNoRows}
	service := NewService(repo)

	_, err := service.GetOrder(context.Background(), mustUUID(t, "11111111-1111-4111-8111-111111111111"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestCreateOrderValidatesAndStoresSnapshot(t *testing.T) {
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	repo := &stubRepo{}
	service := NewService(repo)

	order, err := service.CreateOrder(context.Background(), CreateOrderInput{
		ProductID:      productID,
		ProductName:    "  Keyboard  ",
		Quantity:       2,
		UnitPriceCents: 7900,
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if !repo.called {
		t.Fatal("repository was not called")
	}
	if !repo.createParams.ID.Valid || repo.createParams.ID.Bytes[6]>>4 != 4 {
		t.Fatalf("order id = %+v, want a valid UUID version 4", repo.createParams.ID)
	}
	if repo.createParams.ProductID != productID {
		t.Fatalf("product id = %s", repo.createParams.ProductID.String())
	}
	if repo.createParams.ProductName != "Keyboard" || repo.createParams.Quantity != 2 || repo.createParams.UnitPriceCents != 7900 {
		t.Fatalf("params = %+v", repo.createParams)
	}
	if order.Status != "pending" || order.ProductName != "Keyboard" {
		t.Fatalf("order = %+v", order)
	}
}

func TestCreateOrderRejectsInvalidInput(t *testing.T) {
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	tests := []struct {
		name  string
		input CreateOrderInput
		want  string
	}{
		{
			name:  "missing product",
			input: CreateOrderInput{ProductName: "Keyboard", Quantity: 1, UnitPriceCents: 1},
			want:  "productId must be a UUID",
		},
		{
			name:  "missing name",
			input: CreateOrderInput{ProductID: productID, ProductName: "   ", Quantity: 1, UnitPriceCents: 1},
			want:  "productName is required",
		},
		{
			name:  "name too long",
			input: CreateOrderInput{ProductID: productID, ProductName: strings.Repeat("a", 256), Quantity: 1, UnitPriceCents: 1},
			want:  "productName must be at most 255 characters",
		},
		{
			name:  "quantity",
			input: CreateOrderInput{ProductID: productID, ProductName: "Keyboard", Quantity: 0, UnitPriceCents: 1},
			want:  "quantity must be greater than 0",
		},
		{
			name:  "price",
			input: CreateOrderInput{ProductID: productID, ProductName: "Keyboard", Quantity: 1, UnitPriceCents: -1},
			want:  "unitPriceCents must be zero or greater",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubRepo{}
			_, err := NewService(repo).CreateOrder(context.Background(), tt.input)

			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Error() != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if repo.called {
				t.Fatal("repository was called")
			}
		})
	}
}

type stubRepo struct {
	orders       []repository.Order
	getErr       error
	order        repository.Order
	findErr      error
	called       bool
	createParams repository.CreateOrderParams
	createErr    error
}

func (s *stubRepo) GetOrders(context.Context) ([]repository.Order, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.orders, nil
}

func (s *stubRepo) FindOrderById(context.Context, pgtype.UUID) (repository.Order, error) {
	if s.findErr != nil {
		return repository.Order{}, s.findErr
	}
	return s.order, nil
}

func (s *stubRepo) CreateOrder(_ context.Context, arg repository.CreateOrderParams) (repository.Order, error) {
	s.called = true
	s.createParams = arg
	if s.createErr != nil {
		return repository.Order{}, s.createErr
	}

	created := sampleOrder(arg.ID, arg.ProductID)
	created.ProductName = arg.ProductName
	created.Quantity = arg.Quantity
	created.UnitPriceCents = arg.UnitPriceCents
	created.Status = "pending"
	return created, nil
}

func sampleOrder(id, productID pgtype.UUID) repository.Order {
	timestamp := pgtype.Timestamptz{Time: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC), Valid: true}
	return repository.Order{
		ID:             id,
		ProductID:      productID,
		ProductName:    "Keyboard",
		Quantity:       2,
		UnitPriceCents: 7900,
		Status:         "pending",
		CreatedAt:      timestamp,
		UpdatedAt:      timestamp,
	}
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("parse uuid %s: %v", value, err)
	}
	return id
}
