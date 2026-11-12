import asyncio
import logging
import os
import signal
import sys

import nats
import psycopg

import consumer
import db

logger = logging.getLogger(__name__)

DEFAULT_DSN = "host=localhost user=app password=app dbname=supply_chain sslmode=disable"
DEFAULT_NATS_URL = "nats://127.0.0.1:4222"


def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
        stream=sys.stdout,
    )
    asyncio.run(run())


def env_string(key: str, fallback: str) -> str:
    value = os.environ.get(key)
    if value:
        return value
    return fallback


async def run() -> None:
    stop = asyncio.Event()
    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        loop.add_signal_handler(sig, stop.set)

    dsn = env_string("GOOSE_DB_STRING", DEFAULT_DSN)
    nats_url = env_string("NATS_URL", DEFAULT_NATS_URL)

    conn = await connect_db(dsn, stop)
    if conn is None:
        return

    nc = await connect_nats(nats_url, stop)
    if nc is None:
        await close_quietly(conn)
        return

    try:
        await consume(nc, conn, dsn, stop)
    finally:
        await nc.drain()


async def connect_db(dsn: str, stop: asyncio.Event) -> psycopg.Connection | None:
    while not stop.is_set():
        try:
            conn = await asyncio.to_thread(db.connect, dsn)
        except psycopg.Error as exc:
            logger.warning("database unavailable, retrying: %s", exc)
            await sleep_or_stop(stop, 1)
            continue
        logger.info("Database connected successfully")
        return conn
    return None


async def connect_nats(nats_url: str, stop: asyncio.Event):
    while not stop.is_set():
        try:
            nc = await nats.connect(
                nats_url,
                name="analytics",
                connect_timeout=2,
                max_reconnect_attempts=-1,
                reconnect_time_wait=1,
            )
        except Exception as exc:
            logger.warning("NATS unavailable, retrying: %s", exc)
            await sleep_or_stop(stop, 1)
            continue
        logger.info("NATS connected")
        return nc
    return None


async def consume(nc, conn: psycopg.Connection, dsn: str, stop: asyncio.Event) -> None:
    try:
        js = nc.jetstream()
        subscription = None
        while subscription is None and not stop.is_set():
            try:
                await consumer.ensure_stream(js)
                subscription = await consumer.subscribe(js)
            except Exception as exc:
                logger.warning("JetStream unavailable, retrying: %s", exc)
                await sleep_or_stop(stop, 1)

        if subscription is None:
            return

        logger.info("consuming %s", ", ".join(consumer.SUBJECTS))
        while not stop.is_set():
            try:
                messages = await subscription.fetch(10, timeout=1)
            except TimeoutError:
                continue
            except Exception as exc:
                logger.warning("fetch failed, retrying: %s", exc)
                await sleep_or_stop(stop, 1)
                continue

            for message in messages:
                if stop.is_set():
                    return
                try:
                    await consumer.handle_message(
                        message,
                        lambda event, connection=conn: db.save_event(connection, event),
                    )
                except psycopg.Error:
                    await close_quietly(conn)
                    conn = await connect_db(dsn, stop)
                    if conn is None:
                        return
    finally:
        if conn is not None:
            await close_quietly(conn)


async def close_quietly(conn) -> None:
    try:
        await asyncio.to_thread(conn.close)
    except psycopg.Error:
        pass


async def sleep_or_stop(stop: asyncio.Event, seconds: float) -> None:
    try:
        await asyncio.wait_for(stop.wait(), timeout=seconds)
    except TimeoutError:
        return


if __name__ == "__main__":
    main()
