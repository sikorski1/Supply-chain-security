from pathlib import Path

import psycopg
from psycopg.types.json import Json

from consumer import OrderEvent

MIGRATION = Path(__file__).resolve().parent / "migrations" / "00001_create_order_events.sql"


def connect(dsn: str) -> psycopg.Connection:
    conn = psycopg.connect(dsn)
    try:
        ensure_schema(conn)
    except Exception:
        conn.close()
        raise
    return conn


def ensure_schema(conn: psycopg.Connection) -> None:
    with conn.cursor() as cursor:
        for statement in schema_statements():
            cursor.execute(statement)
    conn.commit()


def save_event(conn: psycopg.Connection, event: OrderEvent) -> bool:
    try:
        row = conn.execute(
            """
            INSERT INTO order_events (
                event_id,
                order_id,
                event_type,
                occurred_at,
                payload
            )
            VALUES (%s, %s, %s, %s, %s)
            ON CONFLICT (event_id) DO NOTHING
            RETURNING event_id
            """,
            (
                event.event_id,
                event.order_id,
                event.event_type,
                event.occurred_at,
                Json(event.payload),
            ),
        ).fetchone()
        conn.commit()
    except psycopg.Error:
        _rollback(conn)
        raise
    return row is not None


def schema_statements() -> list[str]:
    text = MIGRATION.read_text()
    try:
        up = text.split("-- +goose Up", 1)[1].split("-- +goose Down", 1)[0]
    except IndexError as exc:
        raise RuntimeError(f"invalid migration file: {MIGRATION}") from exc

    statements = []
    for chunk in up.split(";"):
        lines = []
        for line in chunk.splitlines():
            stripped = line.strip()
            if not stripped or stripped.startswith("--"):
                continue
            lines.append(line)
        statement = "\n".join(lines).strip()
        if statement:
            statements.append(statement)
    if not statements:
        raise RuntimeError(f"migration {MIGRATION} has no statements")
    return statements


def _rollback(conn: psycopg.Connection) -> None:
    try:
        conn.rollback()
    except psycopg.Error:
        pass
