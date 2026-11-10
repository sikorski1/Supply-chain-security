package internal

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
)

var ErrNotFound = errors.New("order not found")

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type CreateOrderInput struct {
	ProductID      pgtype.UUID
	ProductName    string
	Quantity       int32
	UnitPriceCents int32
}

type Service interface {
	GetOrders(ctx context.Context) ([]repository.Order, error)
	GetOrder(ctx context.Context, id pgtype.UUID) (repository.Order, error)
	CreateOrder(ctx context.Context, input CreateOrderInput) (repository.Order, error)
}

type svc struct {
	repository repository.Querier
}

func NewService(r repository.Querier) Service {
	return &svc{
		repository: r,
	}
}

func (s *svc) GetOrders(ctx context.Context) ([]repository.Order, error) {
	orders, err := s.repository.GetOrders(ctx)
	if err != nil {
		return nil, err
	}
	if orders == nil {
		return []repository.Order{}, nil
	}
	return orders, nil
}

func (s *svc) GetOrder(ctx context.Context, id pgtype.UUID) (repository.Order, error) {
	order, err := s.repository.FindOrderById(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.Order{}, ErrNotFound
	}
	if err != nil {
		return repository.Order{}, err
	}
	return order, nil
}

func (s *svc) CreateOrder(ctx context.Context, input CreateOrderInput) (repository.Order, error) {
	name := strings.TrimSpace(input.ProductName)
	if !input.ProductID.Valid {
		return repository.Order{}, &ValidationError{Message: "productId must be a UUID"}
	}
	if name == "" {
		return repository.Order{}, &ValidationError{Message: "productName is required"}
	}
	if utf8.RuneCountInString(name) > 255 {
		return repository.Order{}, &ValidationError{Message: "productName must be at most 255 characters"}
	}
	if input.Quantity < 1 {
		return repository.Order{}, &ValidationError{Message: "quantity must be greater than 0"}
	}
	if input.UnitPriceCents < 0 {
		return repository.Order{}, &ValidationError{Message: "unitPriceCents must be zero or greater"}
	}

	id, err := newUUID()
	if err != nil {
		return repository.Order{}, err
	}

	return s.repository.CreateOrder(ctx, repository.CreateOrderParams{
		ID:             id,
		ProductID:      input.ProductID,
		ProductName:    name,
		Quantity:       input.Quantity,
		UnitPriceCents: input.UnitPriceCents,
	})
}

func newUUID() (pgtype.UUID, error) {
	var id pgtype.UUID
	if _, err := rand.Read(id.Bytes[:]); err != nil {
		return pgtype.UUID{}, err
	}
	id.Bytes[6] = (id.Bytes[6] & 0x0f) | 0x40
	id.Bytes[8] = (id.Bytes[8] & 0x3f) | 0x80
	id.Valid = true
	return id, nil
}
