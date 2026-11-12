package postgresql

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/inventory/internal/adapters/postgresql/sqlc"
)

type DB interface {
	repository.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Store struct {
	db DB
}

func New(db DB) *Store {
	return &Store{db: db}
}

func (s *Store) GetProducts(ctx context.Context) ([]repository.GetProductsRow, error) {
	return repository.New(s.db).GetProducts(ctx)
}

func (s *Store) FindProductById(ctx context.Context, id pgtype.UUID) (repository.FindProductByIdRow, error) {
	return repository.New(s.db).FindProductById(ctx, id)
}

func (s *Store) WithinTx(ctx context.Context, fn func(repository.Querier) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := fn(repository.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
