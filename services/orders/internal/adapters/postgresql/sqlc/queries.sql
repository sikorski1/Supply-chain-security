-- name: GetOrders :many
SELECT * FROM orders
ORDER BY created_at DESC;

-- name: GetOrderStats :one
SELECT
    COUNT(*)::int AS total_orders,
    COUNT(*) FILTER (WHERE status = 'pending')::int AS pending_orders,
    COUNT(*) FILTER (WHERE status = 'processing')::int AS processing_orders,
    COUNT(*) FILTER (WHERE status = 'completed')::int AS completed_orders,
    COUNT(*) FILTER (WHERE status = 'failed')::int AS failed_orders
FROM orders;

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