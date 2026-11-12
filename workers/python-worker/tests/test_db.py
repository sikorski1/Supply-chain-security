import os
from datetime import datetime, timezone
from uuid import uuid4

import psycopg
import pytest

from consumer import OrderEvent
from db import save_event, schema_statements
from db import connect as connect_db

DSN = os.environ.get("GOOSE_DB_STRING") or (
    "host=localhost user=app password=app dbname=supply_chain sslmode=disable"
)
WHEN = datetime(2026, 11, 12, 18, 30, tzinfo=timezone.utc)


def test_migration_defines_idempotent_order_events():
    sql = "\n".join(schema_statements())

    assert "CREATE TABLE IF NOT EXISTS order_events" in sql
    assert "event_id UUID PRIMARY KEY" in sql
    assert "'orders.created'" in sql
    assert "'orders.completed'" in sql
    assert "'orders.failed'" in sql
    assert "CREATE INDEX IF NOT EXISTS idx_order_events_order_id" in sql


@pytest.fixture
def conn():
    try:
        connection = connect_db(DSN)
    except psycopg.OperationalError as exc:
        pytest.skip(f"postgres unavailable: {exc}")

    yield connection
    connection.close()


def test_save_event_is_idempotent_by_event_id(conn):
    event_id = uuid4()
    order_id = uuid4()
    first = order_event(event_id, order_id, "orders.created", note="first")
    duplicate = order_event(event_id, order_id, "orders.failed", note="second")

    try:
        assert save_event(conn, first) is True
        assert save_event(conn, duplicate) is False

        rows = conn.execute(
            """
            SELECT order_id, event_type, occurred_at, payload
            FROM order_events
            WHERE event_id = %s
            """,
            (event_id,),
        ).fetchall()
        assert len(rows) == 1
        order_id_value, event_type, occurred_at, payload = rows[0]
        assert order_id_value == order_id
        assert event_type == "orders.created"
        assert occurred_at == WHEN
        assert payload["note"] == "first"
    finally:
        conn.rollback()
        conn.execute("DELETE FROM order_events WHERE event_id = %s", (event_id,))
        conn.commit()


def test_same_order_can_have_distinct_events(conn):
    order_id = uuid4()
    created_id = uuid4()
    completed_id = uuid4()

    try:
        assert save_event(conn, order_event(created_id, order_id, "orders.created")) is True
        assert save_event(conn, order_event(completed_id, order_id, "orders.completed")) is True

        count = conn.execute(
            "SELECT count(*) FROM order_events WHERE event_id IN (%s, %s)",
            (created_id, completed_id),
        ).fetchone()[0]
        assert count == 2
    finally:
        conn.rollback()
        conn.execute(
            "DELETE FROM order_events WHERE event_id IN (%s, %s)",
            (created_id, completed_id),
        )
        conn.commit()


def test_schema_can_be_applied_twice(conn):
    from db import ensure_schema

    ensure_schema(conn)
    ensure_schema(conn)


def order_event(event_id, order_id, event_type, note="created"):
    return OrderEvent(
        event_id=event_id,
        order_id=order_id,
        event_type=event_type,
        occurred_at=WHEN,
        payload={
            "eventId": str(event_id),
            "orderId": str(order_id),
            "occurredAt": WHEN.isoformat(),
            "note": note,
        },
    )
