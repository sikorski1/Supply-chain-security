-- name: FindPaymentByOrderId :one
SELECT * FROM payments WHERE order_id = $1;

-- name: CreatePayment :one
INSERT INTO payments (
    id,
    order_id,
    amount_cents,
    status
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;
