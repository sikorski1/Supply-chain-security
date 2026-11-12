-- +goose Up
CREATE TABLE notifications (
    id UUID PRIMARY KEY,

    event_id UUID NOT NULL UNIQUE,
    order_id UUID NOT NULL,

    type VARCHAR(32) NOT NULL
        CHECK (type IN (
            'orders.created',
            'orders.completed',
            'orders.failed'
        )),

    message TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_created_at
    ON notifications (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS notifications;
