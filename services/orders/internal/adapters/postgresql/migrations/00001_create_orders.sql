-- +goose Up
CREATE TABLE orders (
    id UUID PRIMARY KEY,

    product_id UUID NOT NULL,
    product_name VARCHAR(255) NOT NULL,

    quantity INTEGER NOT NULL
        CHECK (quantity > 0),

    unit_price_cents INTEGER NOT NULL
        CHECK (unit_price_cents >= 0),

    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN (
            'pending',
            'processing',
            'completed',
            'failed'
        )),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_orders_created_at
    ON orders (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS orders;
