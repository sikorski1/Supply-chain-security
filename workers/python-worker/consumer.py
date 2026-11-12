import asyncio
import json
import logging
from dataclasses import dataclass
from datetime import datetime, timezone
from uuid import UUID

import psycopg
from nats.js.api import AckPolicy, ConsumerConfig, DeliverPolicy, RetentionPolicy
from nats.js.errors import BadRequestError, NotFoundError

logger = logging.getLogger(__name__)

STREAM = "ORDERS"
DURABLE = "analytics"
SUBJECTS = ("orders.created", "orders.completed", "orders.failed")


class InvalidEvent(ValueError):
    pass


@dataclass(frozen=True)
class OrderEvent:
    event_id: UUID
    order_id: UUID
    event_type: str
    occurred_at: datetime
    payload: dict


def parse_event(subject: str, data: bytes, *, fallback_time: datetime | None = None) -> OrderEvent:
    if subject not in SUBJECTS:
        raise InvalidEvent(f"unexpected subject {subject}")

    try:
        body = json.loads(data)
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        raise InvalidEvent("payload is not json") from exc
    if not isinstance(body, dict):
        raise InvalidEvent("payload must be a json object")

    return OrderEvent(
        event_id=_uuid_field(body, "eventId"),
        order_id=_uuid_field(body, "orderId"),
        event_type=subject,
        occurred_at=_occurred_at(body.get("occurredAt"), fallback_time),
        payload=body,
    )


async def ensure_stream(js) -> None:
    try:
        info = await js.stream_info(STREAM)
    except NotFoundError:
        try:
            info = await js.add_stream(
                name=STREAM, subjects=["orders.>"], retention=RetentionPolicy.LIMITS
            )
        except BadRequestError:
            info = await js.stream_info(STREAM)
    if info.config.retention != RetentionPolicy.LIMITS or not any(
        subject in ("orders.>", "orders.*") for subject in info.config.subjects
    ):
        raise ValueError("ORDERS must capture orders.* with limits retention")


async def subscribe(js):
    try:
        info = await js.consumer_info(STREAM, DURABLE)
    except NotFoundError:
        pass
    else:
        filters = info.config.filter_subjects or [info.config.filter_subject]
        if info.config.ack_policy != AckPolicy.EXPLICIT or set(filters) != set(SUBJECTS):
            raise ValueError("analytics durable must use expected subjects and explicit acknowledgements")
    return await js.pull_subscribe(
        SUBJECTS[0],
        durable=DURABLE,
        stream=STREAM,
        config=ConsumerConfig(
            filter_subjects=list(SUBJECTS),
            ack_policy=AckPolicy.EXPLICIT,
            deliver_policy=DeliverPolicy.ALL,
        ),
    )


async def handle_message(message, save) -> bool | None:
    """Ack the message only after it has been stored. Nak database failures."""
    try:
        event = parse_event(message.subject, message.data, fallback_time=message_time(message))
    except InvalidEvent as exc:
        logger.warning("discarding invalid order event subject=%s error=%s", message.subject, exc)
        await message.term()
        return None

    try:
        inserted = await asyncio.to_thread(save, event)
    except (psycopg.DataError, psycopg.IntegrityError):
        logger.exception("discarding invalid database row event_id=%s", event.event_id)
        await message.term()
        return None
    except psycopg.Error:
        logger.exception("failed to store order event event_id=%s", event.event_id)
        try:
            await message.nak(delay=1)
        except Exception:
            logger.exception("failed to nak message")
        raise
    await message.ack()
    if inserted:
        logger.info(
            "stored order event event_id=%s order_id=%s event_type=%s",
            event.event_id,
            event.order_id,
            event.event_type,
        )
    else:
        logger.info(
            "duplicate order event ignored event_id=%s event_type=%s",
            event.event_id,
            event.event_type,
        )
    return inserted


def message_time(message) -> datetime | None:
    try:
        timestamp = message.metadata.timestamp
    except Exception:
        return None
    if not isinstance(timestamp, datetime):
        return None
    return timestamp


def _uuid_field(body: dict, name: str) -> UUID:
    value = body.get(name)
    if not isinstance(value, str):
        raise InvalidEvent(f"{name} must be a uuid")
    try:
        return UUID(value)
    except ValueError as exc:
        raise InvalidEvent(f"{name} must be a uuid") from exc


def _occurred_at(value, fallback: datetime | None) -> datetime:
    if value is None:
        when = fallback if fallback is not None else datetime.now(timezone.utc)
        return _as_utc(when)
    if not isinstance(value, str):
        raise InvalidEvent("occurredAt must be an rfc3339 timestamp")
    try:
        parsed = datetime.fromisoformat(value)
    except ValueError as exc:
        raise InvalidEvent("occurredAt must be an rfc3339 timestamp") from exc
    return _as_utc(parsed)


def _as_utc(value: datetime) -> datetime:
    if value.tzinfo is None:
        return value.replace(tzinfo=timezone.utc)
    return value.astimezone(timezone.utc)
