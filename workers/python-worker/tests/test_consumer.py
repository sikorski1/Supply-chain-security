import asyncio
import json
from datetime import datetime, timezone
from uuid import UUID

import psycopg

from consumer import SUBJECTS, handle_message, parse_event

EVENT_ID = "6f1b2c3d-4e5f-4a6b-8c7d-8e9f0a1b2c3d"
ORDER_ID = "11111111-2222-4333-8444-555555555555"
WHEN = datetime(2026, 11, 12, 18, 30, tzinfo=timezone.utc)


class FakeMessage:
    def __init__(self, subject, data, when=None):
        self.subject = subject
        self.data = data
        self.actions = []
        self._when = when

    @property
    def metadata(self):
        if self._when is None:
            raise RuntimeError("no jetstream metadata")
        return type("Metadata", (), {"timestamp": self._when})()

    async def ack(self):
        self.actions.append("ack")

    async def nak(self, delay=None):
        self.actions.append("nak")

    async def term(self):
        self.actions.append("term")


def payload(**overrides):
    body = {
        "eventId": EVENT_ID,
        "orderId": ORDER_ID,
        "occurredAt": "2026-11-12T18:30:00Z",
        "quantity": 2,
    }
    body.update(overrides)
    return json.dumps(body).encode()


def test_parse_reads_each_order_subject():
    for subject in SUBJECTS:
        event = parse_event(subject, payload(), fallback_time=WHEN)

        assert event.event_type == subject
        assert event.event_id == UUID(EVENT_ID)
        assert event.order_id == UUID(ORDER_ID)
        assert event.occurred_at == WHEN
        assert event.payload["quantity"] == 2


def test_parse_uses_subject_as_event_type():
    event = parse_event(
        "orders.created",
        payload(eventType="orders.failed"),
        fallback_time=WHEN,
    )

    assert event.event_type == "orders.created"


def test_parse_normalizes_occurred_at_to_utc():
    naive = parse_event("orders.created", payload(occurredAt="2026-11-12T18:30:00"))
    shifted = parse_event("orders.created", payload(occurredAt="2026-11-12T19:30:00+01:00"))

    assert naive.occurred_at == WHEN
    assert shifted.occurred_at == WHEN


def test_parse_uses_fallback_when_occurred_at_is_missing():
    body = {"eventId": EVENT_ID, "orderId": ORDER_ID}

    event = parse_event("orders.failed", json.dumps(body).encode(), fallback_time=WHEN)

    assert event.occurred_at == WHEN
    assert event.event_type == "orders.failed"


def test_parse_rejects_invalid_messages():
    cases = [
        ("orders.updated", payload(), "unexpected subject"),
        ("orders.created", b"[", "not json"),
        ("orders.created", b"[]", "json object"),
        ("orders.created", payload(eventId=None), "eventId"),
        ("orders.created", payload(eventId="order-1"), "eventId"),
        ("orders.created", payload(orderId=""), "orderId"),
        ("orders.created", payload(occurredAt="yesterday"), "occurredAt"),
        ("orders.created", payload(occurredAt=1), "occurredAt"),
    ]

    for subject, data, expected in cases:
        try:
            parse_event(subject, data, fallback_time=WHEN)
        except ValueError as exc:
            assert expected in str(exc)
        else:
            raise AssertionError(f"expected {expected!r} to be rejected")


def test_ack_happens_only_after_save():
    message = FakeMessage("orders.created", payload())
    calls = []

    def save(event):
        calls.append(event.event_id)
        assert message.actions == []
        return True

    inserted = asyncio.run(handle_message(message, save))

    assert inserted is True
    assert calls == [UUID(EVENT_ID)]
    assert message.actions == ["ack"]


def test_duplicate_event_is_still_acknowledged():
    message = FakeMessage("orders.completed", payload())

    inserted = asyncio.run(handle_message(message, lambda event: False))

    assert inserted is False
    assert message.actions == ["ack"]


def test_database_error_naks_without_ack():
    message = FakeMessage("orders.failed", payload())

    def save(event):
        raise psycopg.OperationalError("connection lost")

    try:
        asyncio.run(handle_message(message, save))
    except psycopg.OperationalError:
        pass
    else:
        raise AssertionError("database error was swallowed")

    assert message.actions == ["nak"]


def test_row_rejected_by_database_is_terminated_without_ack():
    message = FakeMessage("orders.created", payload())

    def save(event):
        raise psycopg.DataError("invalid order event row")

    inserted = asyncio.run(handle_message(message, save))

    assert inserted is None
    assert message.actions == ["term"]


def test_invalid_message_is_terminated_without_saving():
    message = FakeMessage("orders.created", b"not-json")
    saved = []

    inserted = asyncio.run(handle_message(message, saved.append))

    assert inserted is None
    assert saved == []
    assert message.actions == ["term"]


def test_missing_occurred_at_uses_jetstream_timestamp():
    message = FakeMessage(
        "orders.created",
        json.dumps({"eventId": EVENT_ID, "orderId": ORDER_ID}).encode(),
        when=WHEN,
    )
    seen = []

    asyncio.run(handle_message(message, lambda event: seen.append(event) or True))

    assert seen[0].occurred_at == WHEN
    assert message.actions == ["ack"]
