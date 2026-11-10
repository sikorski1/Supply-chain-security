-- name: GetOrders :many
SELECT * FROM orders
ORDER BY created_at DESC;

-- name: FindOrderById :one
SELECT * FROM orders WHERE id = $1;

-- name: CreateOrder :one
INSERT INTO orders (
    id,
    product_id,
    product_name,
    quantity,
    unit_price_cents
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;