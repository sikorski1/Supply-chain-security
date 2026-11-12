-- +goose Up
CREATE TABLE reservations (
    id UUID PRIMARY KEY,

    order_id UUID NOT NULL UNIQUE,
    product_id UUID NOT NULL REFERENCES products (id),

    quantity INTEGER NOT NULL
        CHECK (quantity > 0),

    status VARCHAR(20) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'released')),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reservations_active_product
    ON reservations (product_id)
    WHERE status = 'active';

-- +goose Down
DROP TABLE IF EXISTS reservations;
