package internal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
)

type workflowStore struct {
	*stubRepo
	statuses        []string
	processingError bool
}

func (s *workflowStore) UpdateOrderStatus(_ context.Context, id pgtype.UUID, status string) (repository.Order, error) {
	s.statuses = append(s.statuses, status)
	if status == "processing" && s.processingError {
		return repository.Order{}, context.Canceled
	}
	order := sampleOrder(id, s.createParams.ProductID)
	order.Status = status
	return order, nil
}

type testPublisher struct {
	events []string
	fail   string
}

func (p *testPublisher) Publish(_ context.Context, subject string, order repository.Order) error {
	p.events = append(p.events, subject)
	if subject == p.fail {
		return errors.New("NATS unavailable")
	}
	return nil
}

func TestOrderWorkflow(t *testing.T) {
	for _, mode := range []string{"success", "stock", "payment failed", "payment unavailable", "reservation timeout", "reservation lost response", "status update", "cancelled payment", "publish created", "publish completed", "overflow", "free"} {
		t.Run(mode, func(t *testing.T) {
			repo := &workflowStore{stubRepo: &stubRepo{}}
			pub := &testPublisher{}
			repo.processingError = mode == "status update"
			if strings.HasPrefix(mode, "publish ") {
				pub.fail = "orders." + strings.TrimPrefix(mode, "publish ")
			}
			reserved, released, paid := 0, 0, 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			inventory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == "GET":
					price := 100
					if mode == "overflow" {
						price = 2147483647
					}
					if mode == "free" {
						price = 0
					}
					json.NewEncoder(w).Encode(map[string]any{"id": "22222222-2222-4222-8222-222222222222", "name": "Keyboard", "priceCents": price})
				case strings.HasSuffix(r.URL.Path, "/release"):
					released++
					w.Write([]byte(`{"status":"released"}`))
				default:
					reserved++
					if mode == "stock" {
						http.Error(w, `{"code":"insufficient_stock"}`, 409)
						return
					}
					if mode == "reservation timeout" || (mode == "reservation lost response" && reserved == 1) {
						http.Error(w, "uncertain response", 504)
						return
					}
					w.Write([]byte(`{"status":"active"}`))
				}
			}))
			defer inventory.Close()
			payments := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paid++
				var body struct {
					Amount int `json:"amountCents"`
				}
				json.NewDecoder(r.Body).Decode(&body)
				if body.Amount != 200 {
					t.Errorf("amount = %d, want authoritative price * quantity", body.Amount)
				}
				if mode == "payment unavailable" {
					http.Error(w, "down", 503)
					return
				}
				if mode == "cancelled payment" && paid == 1 {
					cancel()
					return
				}
				status := "completed"
				if mode == "payment failed" {
					status = "failed"
				}
				json.NewEncoder(w).Encode(map[string]string{"status": status})
			}))
			defer payments.Close()
			service := NewWorkflow(NewService(repo), repo, inventory.URL, payments.URL, pub)
			order, err := service.CreateOrder(ctx, CreateOrderInput{
				ProductID: mustUUID(t, "22222222-2222-4222-8222-222222222222"), Quantity: 2,
				ProductName: "untrusted", UnitPriceCents: 1,
			})
			switch mode {
			case "success", "free", "cancelled payment", "reservation lost response":
				if err != nil || order.Status != "completed" || released != 0 {
					t.Fatalf("order=%+v err=%v released=%d", order, err, released)
				}
				if repo.createParams.ProductName != "Keyboard" {
					t.Fatal("client snapshot was trusted")
				}
				if strings.Join(pub.events, ",") != "orders.created,orders.completed" {
					t.Fatal(pub.events)
				}
				if mode == "free" && paid != 0 {
					t.Fatal("zero amount sent to Payments")
				}
			case "overflow":
				if err == nil || reserved != 0 || repo.called {
					t.Fatalf("overflow accepted: %v", err)
				}
			case "reservation timeout":
				if err == nil || released != 0 || reserved != 2 || paid != 0 || repo.statuses[len(repo.statuses)-1] != "processing" {
					t.Fatalf("uncertain reservation needs reconciliation: err=%v released=%d reserved=%d statuses=%v", err, released, reserved, repo.statuses)
				}
			case "payment unavailable":
				if err == nil || released != 0 || paid != 2 || repo.statuses[len(repo.statuses)-1] != "processing" {
					t.Fatalf("uncertain payment must stay reserved for reconciliation: err=%v released=%d paid=%d statuses=%v", err, released, paid, repo.statuses)
				}
			case "publish completed":
				if err == nil || released != 0 || repo.statuses[len(repo.statuses)-1] != "completed" {
					t.Fatalf("paid order compensated: %v %v", err, repo.statuses)
				}
			default:
				if mode == "payment failed" {
					if err != nil || order.Status != "failed" {
						t.Fatalf("order=%+v err=%v", order, err)
					}
				} else if err == nil {
					t.Fatal("expected error")
				}
				if mode == "payment failed" || mode == "payment unavailable" || mode == "cancelled payment" || mode == "reservation timeout" {
					if released != 1 {
						t.Fatalf("released = %d, want 1", released)
					}
				}
				if len(repo.statuses) == 0 || repo.statuses[len(repo.statuses)-1] != "failed" {
					t.Fatalf("statuses = %v", repo.statuses)
				}
			}
		})
	}
}
