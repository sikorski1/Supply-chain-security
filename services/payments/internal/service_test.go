package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/payments/internal/adapters/postgresql/sqlc"
)

func TestCreatePaymentStoresCompletedPayment(t *testing.T) {
	orderID := mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1")
	repo := &stubRepo{}
	processor := &scriptedProcessor{status: statusCompleted}

	result, err := newService(repo, processor).CreatePayment(context.Background(), CreatePaymentInput{
		OrderID:     orderID,
		AmountCents: 599800,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if !result.Created || processor.calls != 1 || !repo.createCalled {
		t.Fatalf("created = %v, processor calls = %d, createCalled = %v", result.Created, processor.calls, repo.createCalled)
	}
	if !repo.createParams.ID.Valid || repo.createParams.ID.Bytes[6]>>4 != 4 {
		t.Fatalf("payment id = %+v, want a UUID version 4", repo.createParams.ID)
	}
	if repo.createParams.OrderID != orderID || repo.createParams.AmountCents != 599800 || repo.createParams.Status != statusCompleted {
		t.Fatalf("params = %+v", repo.createParams)
	}
	if result.Payment.Status != statusCompleted || result.Payment.AmountCents != 599800 {
		t.Fatalf("payment = %+v", result.Payment)
	}
}

func TestNewServiceCompletesPayment(t *testing.T) {
	repo := &stubRepo{}

	result, err := NewService(repo).CreatePayment(context.Background(), CreatePaymentInput{
		OrderID:     mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1"),
		AmountCents: 599800,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if !result.Created || repo.createParams.Status != statusCompleted {
		t.Fatalf("result = %+v, status = %s", result, repo.createParams.Status)
	}
}

func TestCreatePaymentStoresFailedResult(t *testing.T) {
	repo := &stubRepo{}
	processor := &scriptedProcessor{status: statusFailed}

	result, err := newService(repo, processor).CreatePayment(context.Background(), CreatePaymentInput{
		OrderID:     mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1"),
		AmountCents: 100,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if !result.Created || result.Payment.Status != statusFailed || repo.createParams.Status != statusFailed {
		t.Fatalf("result = %+v, params = %+v", result, repo.createParams)
	}
}

func TestCreatePaymentDoesNotStoreWhenProcessingFails(t *testing.T) {
	repo := &stubRepo{}
	processor := &scriptedProcessor{err: errors.New("processor unavailable")}

	_, err := newService(repo, processor).CreatePayment(context.Background(), CreatePaymentInput{
		OrderID:     mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1"),
		AmountCents: 100,
	})
	if err == nil || repo.createCalled {
		t.Fatalf("error = %v, createCalled = %v", err, repo.createCalled)
	}
}

func TestCreatePaymentRejectsUnexpectedStatus(t *testing.T) {
	repo := &stubRepo{}

	_, err := newService(repo, &scriptedProcessor{status: "pending"}).CreatePayment(context.Background(), CreatePaymentInput{
		OrderID:     mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1"),
		AmountCents: 100,
	})
	if err == nil || repo.createCalled {
		t.Fatalf("error = %v, createCalled = %v", err, repo.createCalled)
	}
}

func TestCreatePaymentReturnsExistingPayment(t *testing.T) {
	orderID := mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1")
	existing := samplePayment(mustUUID(t, "e0c5c1a3-45f0-4b0a-8b12-d44b0b4c3d18"), orderID, 599800, statusCompleted)
	repo := &stubRepo{payment: existing}
	processor := &scriptedProcessor{status: statusFailed}

	result, err := newService(repo, processor).CreatePayment(context.Background(), CreatePaymentInput{
		OrderID:     orderID,
		AmountCents: 599800,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if result.Created || result.Payment.ID != existing.ID || processor.calls != 0 || repo.createCalled {
		t.Fatalf("result = %+v, processor calls = %d, createCalled = %v", result, processor.calls, repo.createCalled)
	}
}

func TestCreatePaymentRejectsDifferentAmount(t *testing.T) {
	orderID := mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1")
	repo := &stubRepo{payment: samplePayment(mustUUID(t, "e0c5c1a3-45f0-4b0a-8b12-d44b0b4c3d18"), orderID, 100, statusCompleted)}

	_, err := NewService(repo).CreatePayment(context.Background(), CreatePaymentInput{
		OrderID:     orderID,
		AmountCents: 200,
	})
	if !errors.Is(err, ErrPaymentExists) {
		t.Fatalf("error = %v, want ErrPaymentExists", err)
	}
	if repo.createCalled {
		t.Fatal("payment was created")
	}
}

func TestCreatePaymentReturnsExistingAfterUniqueViolation(t *testing.T) {
	orderID := mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1")
	existing := samplePayment(mustUUID(t, "e0c5c1a3-45f0-4b0a-8b12-d44b0b4c3d18"), orderID, 599800, statusCompleted)
	repo := &stubRepo{
		createErr:     &pgconn.PgError{Code: "23505"},
		payment:       existing,
		missFirstFind: true,
	}

	result, err := NewService(repo).CreatePayment(context.Background(), CreatePaymentInput{
		OrderID:     orderID,
		AmountCents: 599800,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if result.Created || result.Payment.ID != existing.ID || !repo.createCalled {
		t.Fatalf("result = %+v, createCalled = %v", result, repo.createCalled)
	}
}

func TestCreatePaymentMapsConflictingUniqueViolation(t *testing.T) {
	orderID := mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1")
	repo := &stubRepo{
		createErr:     &pgconn.PgError{Code: "23505"},
		payment:       samplePayment(mustUUID(t, "e0c5c1a3-45f0-4b0a-8b12-d44b0b4c3d18"), orderID, 100, statusFailed),
		missFirstFind: true,
	}

	_, err := NewService(repo).CreatePayment(context.Background(), CreatePaymentInput{
		OrderID:     orderID,
		AmountCents: 200,
	})
	if !errors.Is(err, ErrPaymentExists) || !repo.createCalled {
		t.Fatalf("error = %v, createCalled = %v", err, repo.createCalled)
	}
}

func TestCreatePaymentRejectsInvalidInput(t *testing.T) {
	orderID := mustUUID(t, "a7d94f8e-6b17-4ac9-94f0-70dcb25a42d1")
	tests := []struct {
		name  string
		input CreatePaymentInput
		want  string
	}{
		{name: "order", input: CreatePaymentInput{AmountCents: 1}, want: "orderId must be a UUID"},
		{name: "amount", input: CreatePaymentInput{OrderID: orderID, AmountCents: 0}, want: "amountCents must be greater than 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubRepo{}
			_, err := NewService(repo).CreatePayment(context.Background(), tt.input)

			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Error() != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if repo.findCalled || repo.createCalled {
				t.Fatal("repository was called")
			}
		})
	}
}

type scriptedProcessor struct {
	status string
	err    error
	calls  int
}

func (p *scriptedProcessor) Process(context.Context, pgtype.UUID, int32) (string, error) {
	p.calls++
	if p.err != nil {
		return "", p.err
	}
	return p.status, nil
}

type stubRepo struct {
	payment       repository.Payment
	findErr       error
	findCalls     int
	missFirstFind bool

	findCalled   bool
	createCalled bool
	createParams repository.CreatePaymentParams
	createErr    error
}

func (s *stubRepo) FindPaymentByOrderId(context.Context, pgtype.UUID) (repository.Payment, error) {
	s.findCalled = true
	s.findCalls++
	if s.findErr != nil {
		return repository.Payment{}, s.findErr
	}
	if s.missFirstFind && s.findCalls == 1 {
		return repository.Payment{}, pgx.ErrNoRows
	}
	if !s.payment.ID.Valid {
		return repository.Payment{}, pgx.ErrNoRows
	}
	return s.payment, nil
}

func (s *stubRepo) CreatePayment(_ context.Context, arg repository.CreatePaymentParams) (repository.Payment, error) {
	s.createCalled = true
	s.createParams = arg
	if s.createErr != nil {
		return repository.Payment{}, s.createErr
	}
	return samplePayment(arg.ID, arg.OrderID, arg.AmountCents, arg.Status), nil
}

func samplePayment(id, orderID pgtype.UUID, amountCents int32, status string) repository.Payment {
	return repository.Payment{
		ID:          id,
		OrderID:     orderID,
		AmountCents: amountCents,
		Status:      status,
		CreatedAt:   pgtype.Timestamptz{Time: time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC), Valid: true},
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
