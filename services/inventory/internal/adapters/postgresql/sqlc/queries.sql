-- name: GetProducts :many
SELECT
    p.id,
    p.name,
    p.price_cents,
    GREATEST(
        p.stock_quantity - COALESCE(SUM(r.quantity) FILTER (WHERE r.status = 'active'), 0),
        0
    )::integer AS stock_quantity
FROM products p
LEFT JOIN reservations r ON r.product_id = p.id
GROUP BY p.id
ORDER BY p.name ASC;

-- name: FindProductById :one
SELECT
    p.id,
    p.name,
    p.price_cents,
    GREATEST(
        p.stock_quantity - COALESCE(SUM(r.quantity) FILTER (WHERE r.status = 'active'), 0),
        0
    )::integer AS stock_quantity
FROM products p
LEFT JOIN reservations r ON r.product_id = p.id
WHERE p.id = $1
GROUP BY p.id;

-- name: FindProductForUpdate :one
SELECT id, name, price_cents, stock_quantity
FROM products
WHERE id = $1
FOR UPDATE;

-- name: SumActiveReservationQuantity :one
SELECT COALESCE(SUM(quantity), 0)::integer AS quantity
FROM reservations
WHERE product_id = $1 AND status = 'active';

-- name: FindReservationByOrderId :one
SELECT id, order_id, product_id, quantity, status, created_at, updated_at
FROM reservations
WHERE order_id = $1;

-- name: CreateReservation :one
INSERT INTO reservations (
    id,
    order_id,
    product_id,
    quantity
) VALUES (
    $1, $2, $3, $4
)
RETURNING id, order_id, product_id, quantity, status, created_at, updated_at;

-- name: ReactivateReservation :one
UPDATE reservations
SET
    product_id = $2,
    quantity = $3,
    status = 'active',
    updated_at = NOW()
WHERE order_id = $1
RETURNING id, order_id, product_id, quantity, status, created_at, updated_at;

-- name: ReleaseReservation :one
UPDATE reservations
SET
    status = 'released',
    updated_at = NOW()
WHERE order_id = $1 AND status = 'active'
RETURNING id, order_id, product_id, quantity, status, created_at, updated_at;
