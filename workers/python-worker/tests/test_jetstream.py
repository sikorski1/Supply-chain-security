import asyncio
import json
import os
from uuid import uuid4

import nats
import psycopg
import pytest
from nats.js.api import AckPolicy, ConsumerConfig, DeliverPolicy
from nats.js.errors import NotFoundError

import consumer
from consumer import handle_message
from db import connect as connect_db
from db import save_event

DSN = os.environ.get("GOOSE_DB_STRING") or (
    "host=localhost user=app password=app dbname=supply_chain sslmode=disable"
)
NATS_URL = os.environ.get("NATS_URL") or "nats://127.0.0.1:4222"
TEST_DURABLE = "analytics-test"


def test_worker_consumer_uses_explicit_ack():
    asyncio.run(_worker_consumer())


def test_duplicate_delivery_is_stored_once():
    asyncio.run(_duplicate_delivery())


def test_failed_write_is_redelivered_and_stored_once():
    asyncio.run(_redelivery())


async def _worker_consumer():
    nc = await _connect()
    try:
        js = nc.jetstream()
        await consumer.ensure_stream(js)
        await consumer.ensure_stream(js)
        await consumer.subscribe(js)
        info = await js.consumer_info(consumer.STREAM, consumer.DURABLE)
        assert list(info.config.filter_subjects) == list(consumer.SUBJECTS)
        assert info.config.ack_policy == AckPolicy.EXPLICIT
    finally:
        await nc.close()


async def _duplicate_delivery():
    conn = _database()
    nc = None
    event_id = str(uuid4())
    order_id = str(uuid4())
    try:
        nc = await _connect()
        js = nc.jetstream()
        sub = await _subscription(js)
        await _publish(js, "orders.created", event_id, order_id)
        await _publish(js, "orders.completed", event_id, order_id)
        await js.publish("orders.failed", b"not-json")

        for _ in range(3):
            message = await _fetch_one(sub)
            await handle_message(message, lambda event: save_event(conn, event))

        stored = conn.execute(
            """
            SELECT event_type
            FROM order_events
            WHERE event_id = %s
            """,
            (event_id,),
        ).fetchall()
        assert stored == [("orders.created",)]
        await _fetch_none(sub)
    finally:
        if nc is not None:
            await _delete_consumer(nc)
            await nc.close()
        _delete_event(conn, event_id)
        conn.close()


async def _redelivery():
    conn = _database()
    nc = None
    event_id = str(uuid4())
    order_id = str(uuid4())
    attempts = {"count": 0}

    def save(event):
        attempts["count"] += 1
        if attempts["count"] == 1:
            raise psycopg.OperationalError("database unavailable")
        return save_event(conn, event)

    try:
        nc = await _connect()
        js = nc.jetstream()
        sub = await _subscription(js)
        await _publish(js, "orders.failed", event_id, order_id)

        message = await _fetch_one(sub)
        with pytest.raises(psycopg.OperationalError):
            await handle_message(message, save)

        message = await _fetch_one(sub)
        # A running analytics worker may have already persisted the same event.
        # Redelivery must still retry this writer, acknowledge, and leave one row.
        await handle_message(message, save)
        assert attempts["count"] == 2
        count = conn.execute(
            "SELECT count(*) FROM order_events WHERE event_id = %s",
            (event_id,),
        ).fetchone()[0]
        assert count == 1
    finally:
        if nc is not None:
            await _delete_consumer(nc)
            await nc.close()
        _delete_event(conn, event_id)
        conn.close()


async def _connect():
    try:
        return await nats.connect(
            NATS_URL,
            name="analytics-test",
            connect_timeout=2,
            allow_reconnect=False,
            max_reconnect_attempts=0,
        )
    except Exception as exc:
        pytest.skip(f"nats unavailable: {exc}")


def _database():
    try:
        return connect_db(DSN)
    except psycopg.OperationalError as exc:
        pytest.skip(f"postgres unavailable: {exc}")


async def _subscription(js):
    await consumer.ensure_stream(js)
    try:
        await js.delete_consumer(consumer.STREAM, TEST_DURABLE)
    except NotFoundError:
        pass
    return await js.pull_subscribe(
        "orders.*",
        durable=TEST_DURABLE,
        stream=consumer.STREAM,
        config=ConsumerConfig(deliver_policy=DeliverPolicy.NEW),
    )


async def _delete_consumer(nc):
    if nc is None or nc.is_closed:
        return
    try:
        await nc.jetstream().delete_consumer(consumer.STREAM, TEST_DURABLE)
    except NotFoundError:
        pass


async def _publish(js, subject, event_id, order_id):
    payload = json.dumps(
        {
            "eventId": event_id,
            "orderId": order_id,
            "occurredAt": "2026-11-12T18:30:00Z",
        }
    ).encode()
    await js.publish(subject, payload)


async def _fetch_one(sub):
    last_error = None
    for _ in range(5):
        try:
            messages = await sub.fetch(1, timeout=1)
            return messages[0]
        except TimeoutError as exc:
            last_error = exc
    raise AssertionError("timed out waiting for a message") from last_error


async def _fetch_none(sub):
    try:
        messages = await sub.fetch(1, timeout=1)
    except TimeoutError:
        return
    raise AssertionError(f"expected no message, got {messages[0].data!r}")


def _delete_event(conn, event_id):
    try:
        conn.rollback()
        conn.execute("DELETE FROM order_events WHERE event_id = %s", (event_id,))
        conn.commit()
    except psycopg.Error:
        pass
