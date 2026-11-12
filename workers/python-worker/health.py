"""Check the worker's schema and durable consumer without changing either."""
import asyncio
import os

import nats
import psycopg


async def main():
    with psycopg.connect(os.environ["GOOSE_DB_STRING"], connect_timeout=2) as conn:
        conn.execute("SELECT 1 FROM order_events LIMIT 0")
    nc = await nats.connect(os.environ["NATS_URL"], connect_timeout=2, allow_reconnect=False)
    try:
        await nc.jetstream().consumer_info("ORDERS", "analytics")
    finally:
        await nc.close()


if __name__ == "__main__":
    asyncio.run(main())
