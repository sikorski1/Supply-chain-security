-- +goose Up

CREATE TABLE payments (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL UNIQUE,
    amount_cents INTEGER NOT NULL
        CHECK (amount_cents > 0),
    status VARCHAR(20) NOT NULL
        CHECK (status IN ('completed', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down

DROP TABLE IF EXISTS payments;
