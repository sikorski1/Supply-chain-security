package internal

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/inventory/internal/adapters/postgresql/sqlc"
)

const (
	reservationActive   = "active"
	reservationReleased = "released"
)

var (
	ErrProductNotFound     = errors.New("product not found")
	ErrReservationNotFound = errors.New("reservation not found")
	ErrInsufficientStock   = errors.New("insufficient stock")
	ErrReservationExists   = errors.New("reservation already exists")
)

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type CreateReservationInput struct {
	OrderID   pgtype.UUID
	ProductID pgtype.UUID
	Quantity  int32
}

type ReservationResult struct {
	Reservation repository.Reservation
	Created     bool
}

type Repository interface {
	GetProducts(ctx context.Context) ([]repository.GetProductsRow, error)
	FindProductById(ctx context.Context, id pgtype.UUID) (repository.FindProductByIdRow, error)
	WithinTx(ctx context.Context, fn func(repository.Querier) error) error
}

type Service interface {
	GetProducts(ctx context.Context) ([]repository.Product, error)
	GetProduct(ctx context.Context, id pgtype.UUID) (repository.Product, error)
	CreateReservation(ctx context.Context, input CreateReservationInput) (ReservationResult, error)
	ReleaseReservation(ctx context.Context, orderID pgtype.UUID) (repository.Reservation, error)
}

type svc struct {
	repository Repository
}

func NewService(r Repository) Service {
	return &svc{repository: r}
}

func (s *svc) GetProducts(ctx context.Context) ([]repository.Product, error) {
	rows, err := s.repository.GetProducts(ctx)
	if err != nil {
		return nil, err
	}

	products := make([]repository.Product, 0, len(rows))
	for _, row := range rows {
		products = append(products, toProduct(row.ID, row.Name, row.PriceCents, row.StockQuantity))
	}
	return products, nil
}

func (s *svc) GetProduct(ctx context.Context, id pgtype.UUID) (repository.Product, error) {
	if !id.Valid {
		return repository.Product{}, &ValidationError{Message: "id must be a UUID"}
	}

	row, err := s.repository.FindProductById(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.Product{}, ErrProductNotFound
	}
	if err != nil {
		return repository.Product{}, err
	}
	return toProduct(row.ID, row.Name, row.PriceCents, row.StockQuantity), nil
}

func (s *svc) CreateReservation(ctx context.Context, input CreateReservationInput) (ReservationResult, error) {
	if !input.OrderID.Valid {
		return ReservationResult{}, &ValidationError{Message: "orderId must be a UUID"}
	}
	if !input.ProductID.Valid {
		return ReservationResult{}, &ValidationError{Message: "productId must be a UUID"}
	}
	if input.Quantity < 1 {
		return ReservationResult{}, &ValidationError{Message: "quantity must be greater than 0"}
	}

	var result ReservationResult
	err := s.repository.WithinTx(ctx, func(q repository.Querier) error {
		product, err := q.FindProductForUpdate(ctx, input.ProductID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProductNotFound
		}
		if err != nil {
			return err
		}

		existing, err := q.FindReservationByOrderId(ctx, input.OrderID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		found := err == nil

		if found && existing.Status == reservationActive {
			if existing.ProductID == input.ProductID && existing.Quantity == input.Quantity {
				result.Reservation = existing
				return nil
			}
			return ErrReservationExists
		}
		if found && existing.Status != reservationReleased {
			return fmt.Errorf("unexpected reservation status %q", existing.Status)
		}

		reserved, err := q.SumActiveReservationQuantity(ctx, input.ProductID)
		if err != nil {
			return err
		}
		available := int64(product.StockQuantity) - int64(reserved)
		if int64(input.Quantity) > available {
			return ErrInsufficientStock
		}

		if found {
			updated, err := q.ReactivateReservation(ctx, repository.ReactivateReservationParams{
				OrderID:   input.OrderID,
				ProductID: input.ProductID,
				Quantity:  input.Quantity,
			})
			if err != nil {
				return err
			}
			result.Reservation = updated
			return nil
		}

		id, err := newUUID()
		if err != nil {
			return err
		}
		created, err := q.CreateReservation(ctx, repository.CreateReservationParams{
			ID:        id,
			OrderID:   input.OrderID,
			ProductID: input.ProductID,
			Quantity:  input.Quantity,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return ErrReservationExists
			}
			return err
		}
		result.Reservation = created
		result.Created = true
		return nil
	})
	if err != nil {
		return ReservationResult{}, err
	}
	return result, nil
}

func (s *svc) ReleaseReservation(ctx context.Context, orderID pgtype.UUID) (repository.Reservation, error) {
	if !orderID.Valid {
		return repository.Reservation{}, &ValidationError{Message: "orderId must be a UUID"}
	}

	var released repository.Reservation
	err := s.repository.WithinTx(ctx, func(q repository.Querier) error {
		existing, err := q.FindReservationByOrderId(ctx, orderID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrReservationNotFound
		}
		if err != nil {
			return err
		}
		if existing.Status == reservationReleased {
			released = existing
			return nil
		}
		if existing.Status != reservationActive {
			return fmt.Errorf("unexpected reservation status %q", existing.Status)
		}

		if _, err := q.FindProductForUpdate(ctx, existing.ProductID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProductNotFound
			}
			return err
		}

		updated, err := q.ReleaseReservation(ctx, orderID)
		if err != nil {
			return err
		}
		released = updated
		return nil
	})
	if err != nil {
		return repository.Reservation{}, err
	}
	return released, nil
}

func toProduct(id pgtype.UUID, name string, priceCents, stockQuantity int32) repository.Product {
	return repository.Product{
		ID:            id,
		Name:          name,
		PriceCents:    priceCents,
		StockQuantity: stockQuantity,
	}
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
