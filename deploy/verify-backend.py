"""Run inside Analytics on a test stack: compose exec -T analytics python < this_file.

Uses temporary products and removes their database records after checking the flow.
Exercises internal endpoints directly only to verify idempotency; orders use Gateway.
"""
import asyncio
from concurrent.futures import ThreadPoolExecutor
import json
import os
import time
from urllib.error import HTTPError
from urllib.request import Request, urlopen
from uuid import uuid4

import nats
import psycopg

GATEWAY = "http://api-gateway:8080"
INVENTORY = "http://inventory:8082"
PAYMENTS = "http://payments:8083"


def request(method, path, body=None, base=GATEWAY):
    data = None if body is None else json.dumps(body).encode()
    req = Request(base + path, data=data, method=method, headers={"Content-Type": "application/json"})
    try:
        response = urlopen(req, timeout=30)
    except HTTPError as exc:
        response = exc
    with response:
        raw = response.read()
        return response.status, json.loads(raw) if raw else None


def wait_for(check, message):
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if check():
            return
        time.sleep(0.2)
    raise AssertionError(message)


async def publish_duplicates(payload):
    nc = await nats.connect(os.environ["NATS_URL"])
    try:
        js = nc.jetstream()
        for _ in range(2):
            await js.publish(payload["type"], json.dumps(payload).encode())
        await js.publish("orders.failed", b"\xff")
        for _ in range(100):
            infos = [await js.consumer_info("ORDERS", name) for name in ("analytics", "notifications")]
            if all(info.num_pending == 0 and info.num_ack_pending == 0 for info in infos):
                assert all(info.config.ack_policy == "explicit" for info in infos)
                return
            await asyncio.sleep(0.1)
        raise AssertionError("workers did not acknowledge duplicate/invalid events")
    finally:
        await nc.close()


def main():
    product, scarce, free = [str(uuid4()) for _ in range(3)]
    with psycopg.connect(os.environ["GOOSE_DB_STRING"], autocommit=True) as conn:
        def scalar(sql, params=()):
            return conn.execute(sql, params).fetchone()[0]

        def worker_counts(order_id):
            return (scalar("SELECT count(*) FROM notifications WHERE order_id=%s", (order_id,)),
                    scalar("SELECT count(*) FROM order_events WHERE order_id=%s", (order_id,)))

        try:
            conn.execute("INSERT INTO products VALUES (%s,'Integration keyboard',1200,10),(%s,'Scarce item',100,3),(%s,'Free item',0,3)", (product, scarce, free))
            assert request("GET", "/health")[0] == 200
            for path in ("/internal/payments", "/internal/reservations", f"/internal/products/{product}"):
                assert request("GET", path)[0] == 404, path
                assert request("POST", path, {})[0] == 404, path
            _, stats_before = request("GET", "/api/stats")
            assert request("POST", "/api/stats", {})[0] == 405
            assert set(stats_before) == {"totalOrders", "pendingOrders", "processingOrders", "completedOrders", "failedOrders"}
            assert request("POST", "/api/orders", {"productId": str(uuid4()), "quantity": 1})[0] == 404
            status, order = request("POST", "/api/orders", {"productId": product, "quantity": 2, "productName": "untrusted", "unitPriceCents": 1})
            assert status == 201 and order["status"] == "completed", (status, order)
            assert order["productName"] == "Integration keyboard" and order["unitPriceCents"] == 1200
            order_id = order["id"]
            _, stats_after = request("GET", "/api/stats")
            assert stats_after["totalOrders"] == stats_before["totalOrders"] + 1
            assert stats_after["completedOrders"] == stats_before["completedOrders"] + 1
            assert request("GET", f"/api/orders/{order_id}")[1] == order
            assert any(item["id"] == order_id for item in request("GET", "/api/orders")[1])
            products = request("GET", "/api/products")[1]
            assert next(item for item in products if item["id"] == product)["stockQuantity"] == 8
            assert scalar("SELECT stock_quantity FROM products WHERE id=%s", (product,)) == 10
            assert conn.execute("SELECT amount_cents,status FROM payments WHERE order_id=%s", (order_id,)).fetchone() == (2400, "completed")
            reservation = {"orderId": order_id, "productId": product, "quantity": 2}
            for _ in range(2):
                assert request("POST", "/internal/reservations", reservation, INVENTORY)[0] == 200
                assert request("POST", "/internal/payments", {"orderId": order_id, "amountCents": 2400}, PAYMENTS)[0] == 200
            assert request("POST", "/internal/payments", {"orderId": order_id, "amountCents": 1}, PAYMENTS)[0] == 409
            assert scalar("SELECT count(*) FROM payments WHERE order_id=%s", (order_id,)) == 1
            assert scalar("SELECT count(*) FROM reservations WHERE order_id=%s", (order_id,)) == 1
            wait_for(lambda: worker_counts(order_id) == (2, 2), "both workers must store created and completed")
            payload = conn.execute("SELECT payload FROM order_events WHERE order_id=%s AND event_type='orders.completed'", (order_id,)).fetchone()[0]
            asyncio.run(publish_duplicates(payload))
            assert worker_counts(order_id) == (2, 2), "duplicate events inserted twice"
            status, error = request("POST", "/api/orders", {"productId": product, "quantity": 99})
            assert status == 409 and error["code"] == "INSUFFICIENT_STOCK", (status, error)
            failed_id = scalar("SELECT id FROM orders WHERE product_id=%s AND status='failed'", (product,))
            wait_for(lambda: worker_counts(failed_id) == (2, 2), "both workers must store failed orders")
            assert scalar("SELECT count(*) FROM payments WHERE order_id=%s", (failed_id,)) == 0
            assert request("POST", "/api/orders", {"productId": free, "quantity": 1})[1]["status"] == "completed"
            with ThreadPoolExecutor(max_workers=8) as executor:
                results = list(executor.map(lambda _: request("POST", "/api/orders", {"productId": scarce, "quantity": 1}), range(8)))
            assert sorted(status for status, _ in results) == [201]*3 + [409]*5, results
            assert scalar("SELECT sum(quantity) FROM reservations WHERE product_id=%s AND status='active'", (scarce,)) == 3
            # Basic release idempotency, including concurrent repeats.
            with ThreadPoolExecutor(max_workers=8) as executor:
                released = list(executor.map(lambda _: request("POST", f"/internal/reservations/{order_id}/release", {}, INVENTORY)[0], range(8)))
            assert released == [200]*8, released
            fixture_ids = (product, scarce, free)
            expected = scalar("SELECT count(*)*2 FROM orders WHERE product_id IN (%s,%s,%s)", fixture_ids)
            wait_for(lambda: scalar("SELECT count(*) FROM order_events WHERE order_id IN (SELECT id FROM orders WHERE product_id IN (%s,%s,%s))", fixture_ids) == expected, "Analytics events missing")
            wait_for(lambda: scalar("SELECT count(*) FROM notifications WHERE order_id IN (SELECT id FROM orders WHERE product_id IN (%s,%s,%s))", fixture_ids) == expected, "Notifications missing")
            print(f"PASS: Gateway routing, complete order flow, authoritative prices, stock concurrency, payment/reservation idempotency, both durable consumers, duplicate and poison-event handling ({expected} events per worker)")
        finally:
            fixture_ids = (product, scarce, free)
            for table in ("notifications", "order_events", "payments", "reservations"):
                conn.execute(f"DELETE FROM {table} WHERE order_id IN (SELECT id FROM orders WHERE product_id IN (%s,%s,%s))", fixture_ids)
            conn.execute("DELETE FROM orders WHERE product_id IN (%s,%s,%s)", fixture_ids)
            conn.execute("DELETE FROM products WHERE id IN (%s,%s,%s)", fixture_ids)


if __name__ == "__main__":
    main()
