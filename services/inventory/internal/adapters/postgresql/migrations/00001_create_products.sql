-- +goose Up
CREATE TABLE products (
    id UUID PRIMARY KEY,

    name VARCHAR(255) NOT NULL,

    price_cents INTEGER NOT NULL
        CHECK (price_cents >= 0),

    stock_quantity INTEGER NOT NULL
        CHECK (stock_quantity >= 0)
);

-- +goose Down
DROP TABLE IF EXISTS products;
