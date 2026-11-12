package postgresql

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
)

type Store struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) UpdateOrderStatus(ctx context.Context, id pgtype.UUID, status string) (repository.Order, error) {
	var order repository.Order
	err := s.db.QueryRow(ctx, `UPDATE orders SET status=$2, updated_at=NOW() WHERE id=$1
        RETURNING id,product_id,product_name,quantity,unit_price_cents,status,created_at,updated_at`, id, status).Scan(
		&order.ID, &order.ProductID, &order.ProductName, &order.Quantity, &order.UnitPriceCents, &order.Status, &order.CreatedAt, &order.UpdatedAt)
	return order, err
}
