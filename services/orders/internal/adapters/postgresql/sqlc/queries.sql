-- name: GetOrders :many
SELECT * FROM orders;

-- name: FindOrderById :one
SELECT * FROM orders WHERE id = $1;