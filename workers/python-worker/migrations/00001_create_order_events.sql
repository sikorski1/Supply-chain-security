-- +goose Up

CREATE TABLE IF NOT EXISTS order_events (
    event_id UUID PRIMARY KEY,
    order_id UUID NOT NULL,
    event_type VARCHAR(32) NOT NULL
        CHECK (event_type IN (
            'orders.created',
            'orders.completed',
            'orders.failed'
        )),
    occurred_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_order_events_order_id
    ON order_events (order_id);

-- +goose Down

DROP TABLE IF EXISTS order_events;
