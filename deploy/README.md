# Backend startup and verification

Run from the repository root with Docker Engine and Docker Compose v2 or newer.

```bash
# Validate and build the backend and frontend images.
docker compose -p supply-chain-review -f deploy/docker-compose.yaml config --quiet
docker compose -p supply-chain-review -f deploy/docker-compose.yaml build

# Build and start the entire backend in one command; wait for readiness.
docker compose -p supply-chain-review -f deploy/docker-compose.yaml up --build -d --wait --wait-timeout 120

# Inspect health and logs.
docker compose -p supply-chain-review -f deploy/docker-compose.yaml ps
docker compose -p supply-chain-review -f deploy/docker-compose.yaml logs --tail 100
curl --fail http://127.0.0.1:8080/health

# Repeat the integration checks. They create and remove their own product/order fixtures.
docker compose -p supply-chain-review -f deploy/docker-compose.yaml exec -T analytics python < deploy/verify-backend.py

# Stop the stack while preserving database and JetStream volumes.
docker compose -p supply-chain-review -f deploy/docker-compose.yaml down
```

The verification stack is left running as project `supply-chain-review`. Another project name can be passed with `-p`; use the same name for every command. To change the host ports, set `GATEWAY_PORT` and `FRONTEND_PORT`, for example `GATEWAY_PORT=18080 FRONTEND_PORT=13000 docker compose ... up --build -d --wait`. The project name also scopes its PostgreSQL and NATS volumes. Default credentials `app/app` are for this local benchmark stack.

Gateway binds loopback port 8080. The frontend binds loopback port 3000 and proxies `/api` to the gateway, so the dashboard is at `http://127.0.0.1:3000`. PostgreSQL, NATS, and internal service endpoints are available inside the Compose network. Loopback addresses in health checks refer to the checked container itself. The database URL is `postgresql://app:app@postgres:5432/supply_chain?sslmode=disable`; the broker URL is `nats://nats:4222`.

## Service configuration and behavior

| Service | Internal address | Persistence |
| --- | --- | --- |
| API Gateway | `api-gateway:8080` | None |
| Frontend | `frontend:8080` | None |
| Orders | `orders:8081` | `orders`; `orders_goose_db_version` |
| Inventory | `inventory:8082` | `products`, `reservations`; `inventory_goose_db_version` |
| Payments | `payments:8083` | `payments`; `payments_goose_db_version` |
| Notifications, Node.js | No HTTP listener | `notifications`, unique `event_id` |
| Analytics, Python | No HTTP listener | `order_events`, primary key `event_id` |

The three Go services use pgx connection pools. Each embeds and applies its own Goose SQL migrations before serving HTTP, using a distinct history table so migration version 1 in one service cannot suppress another service's migration. The workers retain their existing idempotent, owned schema initialization. Each application image builds from its own service directory. The frontend image serves the production build and proxies API requests to the gateway. Go builds and dependency installs use pinned versions or lockfiles; Node runtime receives production dependencies and compiled JavaScript. Runtime applications run as non-root users.

Gateway forwards GET products, GET order statistics, and GET/POST orders, including order detail. Internal product lookup, reservation/release, and payment routes remain private.

Order creation keeps the existing request fields and response shape. Inventory supplies the authoritative name and price. Orders stores the pending order, publishes `orders.created`, moves it to processing, reserves stock by order ID, calls Payments with the calculated total, records the final status, and publishes `orders.completed` or `orders.failed`. Free products complete without sending an invalid zero amount to Payments. Inventory retains its existing availability calculation: physical stock minus active reservations. Successful orders keep active reservations; confirmed failures release them. Repeated payment and reservation requests use unique order IDs, and repeated releases are idempotent, including concurrent calls.

The shared file-backed `ORDERS` JetStream stream captures `orders.>` with limits retention. Separate `notifications` and `analytics` durable consumers each receive `orders.created`, `orders.completed`, and `orders.failed`, using explicit acknowledgements after database persistence. Independent consumers and limits retention allow both workers to receive the same events ([NATS documentation](https://docs.nats.io/nats-concepts/jetstream/streams)). Events contain UUID `eventId` and `orderId`, matching subject/type, RFC3339 `occurredAt`, and the order snapshot. Publisher retries reuse the event ID as `Nats-Msg-Id`; both workers deduplicate persisted records by event ID. Temporary persistence errors are retried; malformed messages are terminated.

## Verification results

Verified on 2026-11-12 using Docker Engine 29.5.2 and Compose 5.1.4:

- All four Go suites passed with `go test -race ./...`; `go vet ./...` passed in each module.
- Notifications: 17 tests passed; TypeScript production build and type check passed.
- Analytics: all 20 tests passed with real PostgreSQL and JetStream, without skips. The integration test consumer can coexist with the running worker.
- All six Docker images built successfully from their independent contexts; Compose validation passed.
- All eight containers reached healthy status, including worker schema/consumer checks.
- Gateway order creation verified authoritative product snapshots, reservation-adjusted stock, a completed payment record, completed order status, and both workers' records. The repeatable integration script checks 22 events per worker across its fixture orders.
- Insufficient stock produced HTTP 409 and failed order events; missing products produced HTTP 404. Eight concurrent requests against three available units produced exactly three successful orders and five conflicts.
- Repeated payments/reservations created one record each; repeated concurrent release requests returned HTTP 200. Duplicate and malformed NATS messages did not create extra database records.
- Stopping both workers, creating an order, and restarting the workers verified independent durable replay.
- Restarting PostgreSQL, NATS, and application services preserved payment/order/event records and migration versions: Orders 1, Inventory 2, Payments 1.
- A Payments outage produced HTTP 502 `payment_unconfirmed` and retained the processing order and active reservation for reconciliation.
- Workflow regression tests cover confirmed failed payments with release, lost/cancelled responses and retries, uncertain reservation/payment outcomes, processing-transition failure, overflow, free products, and event-publishing failures.
- An independent read-only review found two failure paths; both were reproduced by regression tests and fixed before the final passing suite.

To rerun the language checks:

```bash
for service in api-gateway services/orders services/inventory services/payments; do
  (cd "$service" && go test -race ./... && go vet ./...) || exit 1
done
(cd workers/notifications && bun install --frozen-lockfile && bun run test && bun run build && bun run typecheck)
```

For the full Python suite on the running stack, use a disposable container with test dependencies and read-only test sources:

```bash
docker run --rm --network supply-chain-review_default --user root   -e GOOSE_DB_STRING='postgresql://app:app@postgres:5432/supply_chain?sslmode=disable'   -e NATS_URL=nats://nats:4222 -e UV_NO_DEV=false   --mount "type=bind,src=$PWD/workers/python-worker/tests,target=/app/tests,readonly"   supply-chain-review-analytics   uv run --frozen --group dev pytest -q -p no:cacheprovider tests
```

## Remaining limits and manual checks

This remains a synchronous example backend with a simulated payment processor. The processor always returns completed in normal operation; confirmed declines are verified through the existing injectable processor and workflow unit tests. No real payment provider, email delivery, SBOM generation, or vulnerability scan was exercised.

Cross-service state changes and event publication are not one atomic transaction. There is no outbox or crash-recovery loop. A process crash, unavailable database during compensation, or exhausted publish retries can require manual order/event reconciliation. This change does not add recovery infrastructure.

A lost reservation/payment response is retried using the same order ID. If it still cannot be confirmed, Orders returns HTTP 502 `reservation_unconfirmed` or `payment_unconfirmed`, including the order ID, and retains processing state and any reservation. Inspect `orders`, `reservations`, and `payments` for that ID before deciding whether to complete the order or release it and mark it failed. Reissuing the internal payment request with the same order ID and amount is idempotent, but may initiate the first simulated payment if none exists. Restore missing final worker events only after confirming the final order state. Do not treat an uncertain payment as a confirmed decline.

Fresh volumes and restart migration behavior were verified. An existing database whose tables were created manually, or through an old shared Goose history, needs a backup and schema comparison before baselining the three new history tables. Non-empty legacy databases were not automatically adopted or modified.

The stack starts with an empty product catalogue. The integration script supplies temporary products; load the desired benchmark catalogue separately. Host Go 1.22.2 selected/downloaded the modules' required Go 1.27.3 toolchain; Docker provides that version explicitly. Images use version tags rather than immutable digests, so byte-identical rebuilds across future upstream tag updates are not guaranteed.

## Modified and added files

- `api-gateway/.dockerignore`
- `api-gateway/Dockerfile`
- `api-gateway/cmd/server/api.go`
- `api-gateway/cmd/server/api_test.go`
- `deploy/README.md`
- `deploy/docker-compose.yaml`
- `deploy/verify-backend.py`
- `services/inventory/.dockerignore`
- `services/inventory/Dockerfile`
- `services/inventory/cmd/server/api.go`
- `services/inventory/cmd/server/main.go`
- `services/inventory/go.mod`
- `services/inventory/go.sum`
- `services/inventory/internal/adapters/postgresql/migrate.go`
- `services/inventory/internal/service.go`
- `services/orders/.dockerignore`
- `services/orders/Dockerfile`
- `services/orders/cmd/server/api.go`
- `services/orders/cmd/server/main.go`
- `services/orders/go.mod`
- `services/orders/go.sum`
- `services/orders/internal/adapters/postgresql/migrate.go`
- `services/orders/internal/adapters/postgresql/store.go`
- `services/orders/internal/events.go`
- `services/orders/internal/handlers.go`
- `services/orders/internal/workflow.go`
- `services/orders/internal/workflow_test.go`
- `services/payments/.dockerignore`
- `services/payments/Dockerfile`
- `services/payments/cmd/server/api.go`
- `services/payments/cmd/server/main.go`
- `services/payments/go.mod`
- `services/payments/go.sum`
- `services/payments/internal/adapters/postgresql/migrate.go`
- `workers/notifications/.dockerignore`
- `workers/notifications/Dockerfile`
- `workers/notifications/src/db.ts`
- `workers/notifications/src/health.ts`
- `workers/notifications/src/index.ts`
- `workers/notifications/src/notifications.ts`
- `workers/python-worker/Dockerfile`
- `workers/python-worker/consumer.py`
- `workers/python-worker/health.py`
- `workers/python-worker/tests/test_consumer.py`
- `workers/python-worker/tests/test_jetstream.py`
