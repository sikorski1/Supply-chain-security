package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/inventory/internal/adapters/postgresql/sqlc"
)

func TestGetProductsReturnsEmptySlice(t *testing.T) {
	service := NewService(&stubRepo{})

	products, err := service.GetProducts(context.Background())
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}
	if products == nil || len(products) != 0 {
		t.Fatalf("products = %#v, want empty slice", products)
	}
}

func TestGetProductNotFound(t *testing.T) {
	service := NewService(&stubRepo{findProductErr: pgx.ErrNoRows})

	_, err := service.GetProduct(context.Background(), mustUUID(t, "22222222-2222-4222-8222-222222222222"))
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("error = %v, want ErrProductNotFound", err)
	}
}

func TestCreateReservationHoldsAvailableStock(t *testing.T) {
	orderID := mustUUID(t, "11111111-1111-4111-8111-111111111111")
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	repo := &stubRepo{
		locked:      sampleProduct(productID, "Keyboard", 7900, 10),
		reservedQty: 4,
	}
	service := NewService(repo)

	result, err := service.CreateReservation(context.Background(), CreateReservationInput{
		OrderID:   orderID,
		ProductID: productID,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	if !result.Created {
		t.Fatal("Created = false, want true")
	}
	if !repo.createParams.ID.Valid || repo.createParams.ID.Bytes[6]>>4 != 4 {
		t.Fatalf("reservation id = %+v, want a UUID version 4", repo.createParams.ID)
	}
	if repo.createParams.OrderID != orderID || repo.createParams.ProductID != productID || repo.createParams.Quantity != 2 {
		t.Fatalf("params = %+v", repo.createParams)
	}
	if result.Reservation.Status != reservationActive {
		t.Fatalf("status = %s", result.Reservation.Status)
	}
}

func TestCreateReservationRejectsInsufficientStock(t *testing.T) {
	repo := &stubRepo{
		locked:      sampleProduct(mustUUID(t, "22222222-2222-4222-8222-222222222222"), "Keyboard", 7900, 5),
		reservedQty: 4,
	}

	_, err := NewService(repo).CreateReservation(context.Background(), CreateReservationInput{
		OrderID:   mustUUID(t, "11111111-1111-4111-8111-111111111111"),
		ProductID: mustUUID(t, "22222222-2222-4222-8222-222222222222"),
		Quantity:  2,
	})
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("error = %v, want ErrInsufficientStock", err)
	}
	if repo.createCalled || repo.reactivateCalled {
		t.Fatal("reservation was written")
	}
}

func TestCreateReservationIsIdempotentForTheSameRequest(t *testing.T) {
	orderID := mustUUID(t, "11111111-1111-4111-8111-111111111111")
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	existing := sampleReservation(mustUUID(t, "33333333-3333-4333-8333-333333333333"), orderID, productID, 2, reservationActive)
	repo := &stubRepo{
		locked:      sampleProduct(productID, "Keyboard", 7900, 10),
		reservation: existing,
	}

	result, err := NewService(repo).CreateReservation(context.Background(), CreateReservationInput{
		OrderID:   orderID,
		ProductID: productID,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	if result.Created || result.Reservation.ID != existing.ID || repo.createCalled {
		t.Fatalf("result = %+v, createCalled = %v", result, repo.createCalled)
	}
}

func TestCreateReservationConflictsWhenOrderReservesSomethingElse(t *testing.T) {
	orderID := mustUUID(t, "11111111-1111-4111-8111-111111111111")
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	repo := &stubRepo{
		locked:      sampleProduct(productID, "Keyboard", 7900, 10),
		reservation: sampleReservation(mustUUID(t, "33333333-3333-4333-8333-333333333333"), orderID, productID, 1, reservationActive),
	}

	_, err := NewService(repo).CreateReservation(context.Background(), CreateReservationInput{
		OrderID:   orderID,
		ProductID: productID,
		Quantity:  2,
	})
	if !errors.Is(err, ErrReservationExists) {
		t.Fatalf("error = %v, want ErrReservationExists", err)
	}
}

func TestCreateReservationReactivatesReleasedReservation(t *testing.T) {
	orderID := mustUUID(t, "11111111-1111-4111-8111-111111111111")
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	repo := &stubRepo{
		locked:      sampleProduct(productID, "Keyboard", 7900, 5),
		reservation: sampleReservation(mustUUID(t, "33333333-3333-4333-8333-333333333333"), orderID, productID, 1, reservationReleased),
	}

	result, err := NewService(repo).CreateReservation(context.Background(), CreateReservationInput{
		OrderID:   orderID,
		ProductID: productID,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	if result.Created || !repo.reactivateCalled || repo.createCalled {
		t.Fatalf("created = %v, reactivate = %v, create = %v", result.Created, repo.reactivateCalled, repo.createCalled)
	}
	if repo.reactivateParams.Quantity != 2 || repo.reactivateParams.ProductID != productID {
		t.Fatalf("params = %+v", repo.reactivateParams)
	}
}

func TestCreateReservationMapsUniqueViolation(t *testing.T) {
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	repo := &stubRepo{
		locked:    sampleProduct(productID, "Keyboard", 7900, 5),
		createErr: &pgconn.PgError{Code: "23505"},
	}

	_, err := NewService(repo).CreateReservation(context.Background(), CreateReservationInput{
		OrderID:   mustUUID(t, "11111111-1111-4111-8111-111111111111"),
		ProductID: productID,
		Quantity:  1,
	})
	if !errors.Is(err, ErrReservationExists) {
		t.Fatalf("error = %v, want ErrReservationExists", err)
	}
}

func TestCreateReservationRejectsInvalidInput(t *testing.T) {
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	orderID := mustUUID(t, "11111111-1111-4111-8111-111111111111")
	tests := []struct {
		name  string
		input CreateReservationInput
		want  string
	}{
		{name: "order", input: CreateReservationInput{ProductID: productID, Quantity: 1}, want: "orderId must be a UUID"},
		{name: "product", input: CreateReservationInput{OrderID: orderID, Quantity: 1}, want: "productId must be a UUID"},
		{name: "quantity", input: CreateReservationInput{OrderID: orderID, ProductID: productID, Quantity: 0}, want: "quantity must be greater than 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubRepo{}
			_, err := NewService(repo).CreateReservation(context.Background(), tt.input)

			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Error() != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if repo.txCalled {
				t.Fatal("transaction was started")
			}
		})
	}
}

func TestReleaseReservationReleasesActiveHold(t *testing.T) {
	orderID := mustUUID(t, "11111111-1111-4111-8111-111111111111")
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	repo := &stubRepo{
		locked:      sampleProduct(productID, "Keyboard", 7900, 5),
		reservation: sampleReservation(mustUUID(t, "33333333-3333-4333-8333-333333333333"), orderID, productID, 2, reservationActive),
	}

	released, err := NewService(repo).ReleaseReservation(context.Background(), orderID)
	if err != nil {
		t.Fatalf("ReleaseReservation: %v", err)
	}
	if !repo.releaseCalled || released.Status != reservationReleased {
		t.Fatalf("released = %+v, releaseCalled = %v", released, repo.releaseCalled)
	}
}

func TestReleaseReservationIsIdempotent(t *testing.T) {
	orderID := mustUUID(t, "11111111-1111-4111-8111-111111111111")
	productID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	existing := sampleReservation(mustUUID(t, "33333333-3333-4333-8333-333333333333"), orderID, productID, 2, reservationReleased)
	repo := &stubRepo{reservation: existing}

	released, err := NewService(repo).ReleaseReservation(context.Background(), orderID)
	if err != nil {
		t.Fatalf("ReleaseReservation: %v", err)
	}
	if released.ID != existing.ID || repo.releaseCalled {
		t.Fatalf("released = %+v, releaseCalled = %v", released, repo.releaseCalled)
	}
}

func TestReleaseReservationNotFound(t *testing.T) {
	_, err := NewService(&stubRepo{}).ReleaseReservation(context.Background(), mustUUID(t, "11111111-1111-4111-8111-111111111111"))
	if !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("error = %v, want ErrReservationNotFound", err)
	}
}

type stubRepo struct {
	products       []repository.GetProductsRow
	product        repository.FindProductByIdRow
	findProductErr error

	locked  repository.Product
	lockErr error

	reservation        repository.Reservation
	findReservationErr error

	reservedQty int32

	createCalled     bool
	createParams     repository.CreateReservationParams
	createErr        error
	reactivateCalled bool
	reactivateParams repository.ReactivateReservationParams
	releaseCalled    bool

	txCalled bool
}

func (s *stubRepo) GetProducts(context.Context) ([]repository.GetProductsRow, error) {
	return s.products, nil
}

func (s *stubRepo) FindProductById(context.Context, pgtype.UUID) (repository.FindProductByIdRow, error) {
	if s.findProductErr != nil {
		return repository.FindProductByIdRow{}, s.findProductErr
	}
	return s.product, nil
}

func (s *stubRepo) WithinTx(_ context.Context, fn func(repository.Querier) error) error {
	s.txCalled = true
	return fn(s)
}

func (s *stubRepo) FindProductForUpdate(context.Context, pgtype.UUID) (repository.Product, error) {
	if s.lockErr != nil {
		return repository.Product{}, s.lockErr
	}
	if !s.locked.ID.Valid {
		return repository.Product{}, pgx.ErrNoRows
	}
	return s.locked, nil
}

func (s *stubRepo) FindReservationByOrderId(context.Context, pgtype.UUID) (repository.Reservation, error) {
	if s.findReservationErr != nil {
		return repository.Reservation{}, s.findReservationErr
	}
	if !s.reservation.ID.Valid {
		return repository.Reservation{}, pgx.ErrNoRows
	}
	return s.reservation, nil
}

func (s *stubRepo) SumActiveReservationQuantity(context.Context, pgtype.UUID) (int32, error) {
	return s.reservedQty, nil
}

func (s *stubRepo) CreateReservation(_ context.Context, arg repository.CreateReservationParams) (repository.Reservation, error) {
	s.createCalled = true
	s.createParams = arg
	if s.createErr != nil {
		return repository.Reservation{}, s.createErr
	}
	return sampleReservation(arg.ID, arg.OrderID, arg.ProductID, arg.Quantity, reservationActive), nil
}

func (s *stubRepo) ReactivateReservation(_ context.Context, arg repository.ReactivateReservationParams) (repository.Reservation, error) {
	s.reactivateCalled = true
	s.reactivateParams = arg
	return sampleReservation(s.reservation.ID, arg.OrderID, arg.ProductID, arg.Quantity, reservationActive), nil
}

func (s *stubRepo) ReleaseReservation(context.Context, pgtype.UUID) (repository.Reservation, error) {
	s.releaseCalled = true
	released := s.reservation
	released.Status = reservationReleased
	return released, nil
}

func sampleProduct(id pgtype.UUID, name string, priceCents, stock int32) repository.Product {
	return repository.Product{
		ID:            id,
		Name:          name,
		PriceCents:    priceCents,
		StockQuantity: stock,
	}
}

func sampleReservation(id, orderID, productID pgtype.UUID, quantity int32, status string) repository.Reservation {
	timestamp := pgtype.Timestamptz{Time: time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC), Valid: true}
	return repository.Reservation{
		ID:        id,
		OrderID:   orderID,
		ProductID: productID,
		Quantity:  quantity,
		Status:    status,
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
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
