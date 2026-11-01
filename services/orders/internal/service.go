package internal

import (
	"context"

	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
)

type Service interface {
	GetOrders(ctx context.Context) ([]repository.Order, error)
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
	return s.repository.GetOrders(ctx)
}