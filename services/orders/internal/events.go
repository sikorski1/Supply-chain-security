package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
)

type EventPublisher struct {
	conn *nats.Conn
	js   nats.JetStreamContext
}

func NewEventPublisher(url string) (*EventPublisher, error) {
	conn, err := nats.Connect(url, nats.Name("orders"), nats.Timeout(3*time.Second), nats.MaxReconnects(-1), nats.ReconnectWait(time.Second), nats.DrainTimeout(5*time.Second))
	if err != nil {
		return nil, err
	}
	js, err := conn.JetStream(nats.MaxWait(2 * time.Second))
	if err != nil {
		conn.Close()
		return nil, err
	}
	info, err := js.StreamInfo("ORDERS")
	if errors.Is(err, nats.ErrStreamNotFound) {
		info, err = js.AddStream(&nats.StreamConfig{Name: "ORDERS", Subjects: []string{"orders.>"}, Storage: nats.FileStorage, Retention: nats.LimitsPolicy})
		// Another service can create the same stream concurrently.
		if err != nil {
			info, err = js.StreamInfo("ORDERS")
		}
	}
	if err == nil {
		compatible := false
		for _, subject := range info.Config.Subjects {
			if subject == "orders.>" || subject == "orders.*" {
				compatible = true
			}
		}
		if !compatible || info.Config.Retention != nats.LimitsPolicy {
			err = fmt.Errorf("ORDERS must capture orders.* with limits retention")
		}
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &EventPublisher{conn: conn, js: js}, nil
}

func (p *EventPublisher) Close() { _ = p.conn.Drain(); p.conn.Close() }

func (p *EventPublisher) Publish(ctx context.Context, subject string, order repository.Order) error {
	id, err := newUUID()
	if err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{
		"eventId": id.String(), "orderId": order.ID.String(), "type": subject,
		"occurredAt": time.Now().UTC().Format(time.RFC3339Nano),
		"productId":  order.ProductID.String(), "quantity": order.Quantity,
		"unitPriceCents": order.UnitPriceCents, "status": order.Status,
	})
	if err != nil {
		return err
	}
	message := nats.NewMsg(subject)
	message.Data = data
	message.Header.Set(nats.MsgIdHdr, id.String())
	// Reuse the event and message ID when the publish acknowledgement is lost.
	for attempt := 0; attempt < 3; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err = p.js.PublishMsg(message, nats.Context(attemptCtx))
		cancel()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("publish %s for order %s: %w", subject, order.ID.String(), err)
}
