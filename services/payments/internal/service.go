package internal

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/payments/internal/adapters/postgresql/sqlc"
)

const (
	statusCompleted = "completed"
	statusFailed    = "failed"
)

var ErrPaymentExists = errors.New("payment already exists")

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type CreatePaymentInput struct {
	OrderID     pgtype.UUID
	AmountCents int32
}

type PaymentResult struct {
	Payment repository.Payment
	Created bool
}

// processor decides a payment result. Production always completes the payment.
// Tests pass another processor when they need a failed payment.
type processor interface {
	Process(ctx context.Context, orderID pgtype.UUID, amountCents int32) (string, error)
}

type succeedingProcessor struct{}

func (succeedingProcessor) Process(context.Context, pgtype.UUID, int32) (string, error) {
	return statusCompleted, nil
}

type Service interface {
	CreatePayment(ctx context.Context, input CreatePaymentInput) (PaymentResult, error)
}

type svc struct {
	repository repository.Querier
	processor  processor
}

func NewService(r repository.Querier) Service {
	return newService(r, succeedingProcessor{})
}

func newService(r repository.Querier, processor processor) Service {
	return &svc{
		repository: r,
		processor:  processor,
	}
}

func (s *svc) CreatePayment(ctx context.Context, input CreatePaymentInput) (PaymentResult, error) {
	if !input.OrderID.Valid {
		return PaymentResult{}, &ValidationError{Message: "orderId must be a UUID"}
	}
	if input.AmountCents < 1 {
		return PaymentResult{}, &ValidationError{Message: "amountCents must be greater than 0"}
	}

	existing, err := s.repository.FindPaymentByOrderId(ctx, input.OrderID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return PaymentResult{}, err
	}
	if err == nil {
		return samePayment(existing, input.AmountCents)
	}

	status, err := s.processor.Process(ctx, input.OrderID, input.AmountCents)
	if err != nil {
		return PaymentResult{}, err
	}
	if status != statusCompleted && status != statusFailed {
		return PaymentResult{}, fmt.Errorf("unexpected payment status %q", status)
	}

	id, err := newUUID()
	if err != nil {
		return PaymentResult{}, err
	}

	created, err := s.repository.CreatePayment(ctx, repository.CreatePaymentParams{
		ID:          id,
		OrderID:     input.OrderID,
		AmountCents: input.AmountCents,
		Status:      status,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			existing, findErr := s.repository.FindPaymentByOrderId(ctx, input.OrderID)
			if findErr != nil {
				return PaymentResult{}, findErr
			}
			return samePayment(existing, input.AmountCents)
		}
		return PaymentResult{}, err
	}

	return PaymentResult{Payment: created, Created: true}, nil
}

func samePayment(existing repository.Payment, amountCents int32) (PaymentResult, error) {
	if existing.AmountCents != amountCents {
		return PaymentResult{}, ErrPaymentExists
	}
	return PaymentResult{Payment: existing, Created: false}, nil
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
