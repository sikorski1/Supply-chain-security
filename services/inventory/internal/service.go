package internal

import (
	repository "github.com/sikorski1/Supply-chain-security/services/inventory/internal/adapters/postgresql/sqlc"
)

type Service interface{}

type svc struct {
	repository repository.Querier
}

func NewService(r repository.Querier) Service {
	return &svc{
		repository: r,
	}
}
