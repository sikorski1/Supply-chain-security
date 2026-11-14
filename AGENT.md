# AGENTS.md

## Project Overview

This repository contains a microservices-based Orders Dashboard application developed as part of a master's thesis on **Software Supply Chain Security in CI/CD pipelines**.

The primary goal is to measure the performance overhead introduced by security mechanisms such as SBOM generation, vulnerability scanning, container image signing, provenance attestations, and policy enforcement.

The application is intentionally small but realistic. It includes multiple programming languages, dependency ecosystems, databases, asynchronous messaging, automated tests, and containerized services.

**The application is a benchmark workload, not a production e-commerce platform.**

## Architecture

The system consists of seven custom application components:

| Component | Technology | Responsibility |
|---|---|---|
| Frontend | React, TypeScript, Vite | Orders Dashboard UI |
| API Gateway | Go | Routes public API requests |
| Orders Service | Go | Order creation and lifecycle management |
| Inventory Service | Go | Product information and stock reservations |
| Payments Service | Go | Simulated payment processing |
| Notifications Worker | Node.js, TypeScript | Processes order events and stores notifications |
| Analytics Worker | Python | Processes and stores order events |

Infrastructure:

- **PostgreSQL** — persistent storage
- **NATS JetStream** — asynchronous event delivery
- **Docker Compose** — local integration environment
- **Kubernetes** — deployment target for CI/CD benchmarking

The seven custom application images are the primary benchmark subjects. PostgreSQL and NATS use official images and are not included in the primary image-level benchmark.

## Service Communication

### Public API

The frontend communicates with the backend through API Gateway.

Supported public endpoints:

- `GET /api/products`
- `GET /api/orders`
- `GET /api/orders/{id}`
- `POST /api/orders`

The API Gateway forwards requests to Orders Service and Inventory Service.

Payments Service is internal and must not be exposed through the public gateway.

Do not introduce a `/api/stats` endpoint.

### Order Processing

The expected order processing flow is:

1. The frontend submits an order through API Gateway.
2. Orders Service retrieves product information from Inventory Service.
3. Inventory Service reserves the requested stock.
4. Orders Service creates and processes the order.
5. Payments Service processes a simulated payment.
6. Orders Service updates the order status.
7. Failed payments result in reservation release.
8. Orders Service publishes relevant order events to NATS JetStream.

The implementation must handle failures and retries without unintentionally duplicating payments or stock reservations.

### Asynchronous Processing

Orders Service publishes events such as:

- `orders.created`
- `orders.completed`
- `orders.failed`

Notifications Worker and Analytics Worker consume these events independently.

Both workers use PostgreSQL for persistence and must handle duplicate event delivery safely.

Messages should be acknowledged only after successful processing.

## Database Conventions

Each service owns its database tables and migrations.

Services may share one PostgreSQL instance, but they must not directly access tables owned by other services.

Use HTTP or NATS for inter-service communication.

Avoid cross-service foreign keys.

Database migrations must not conflict when multiple services share the same database.

Preserve data integrity, especially for stock reservations, payment processing, and event idempotency.

## Engineering Principles

### Keep It Simple

Prefer straightforward, readable implementations over complex architectural patterns.

- Follow existing repository conventions.
- Use the standard library where practical.
- Avoid unnecessary interfaces, factories, middleware, and abstraction layers.
- Do not introduce new frameworks without a clear need.
- Do not add unnecessary endpoints, background jobs, or infrastructure.
- Avoid premature optimization.
- Keep service responsibilities clearly separated.
- Prefer small, focused changes over large refactors.

Do not rewrite working code solely to introduce a different architecture.

### Dependencies

Use real dependencies that are necessary for application functionality.

Do not add unused, artificial, or intentionally vulnerable dependencies to increase SBOM size or vulnerability counts.

Use the existing package managers and lockfiles.

Keep dependency versions reproducible and avoid unnecessary upgrades.

### Testing

Each component should have meaningful automated tests.

Prioritize:

- Business logic
- Input validation
- HTTP routing and error handling
- Database operations
- Idempotency
- Stock reservation concurrency
- Payment processing
- NATS event processing

Do not add artificial tests solely to increase test count or execution time.

Tests should be deterministic and suitable for repeated CI/CD benchmark runs.

## Docker and Local Development

All seven application components should have their own Dockerfiles.

The shared Docker Compose configuration should allow the complete application to run together.

When modifying Docker configuration:

- Preserve reproducible builds.
- Use correct Docker build contexts.
- Use Docker service names for internal communication.
- Ensure environment variables match application configuration.
- Verify health checks and startup dependencies.
- Keep images reasonably small without sacrificing functionality.
- Avoid unnecessary host port exposure.
- Ensure database migrations are handled correctly.

The frontend should communicate with API Gateway through a reverse proxy or an equivalent same-origin configuration.

## CI/CD Security Benchmark

The thesis evaluates cumulative security levels:

| Level | Security mechanisms |
|---|---|
| L0 | Baseline CI/CD pipeline |
| L1 | L0 + SBOM generation using Syft |
| L2 | L1 + vulnerability scanning using Trivy |
| L3 | L2 + container image signing using Cosign |
| L4 | L3 + provenance attestations |
| L5 | L4 + policy enforcement using OPA/Rego |

The benchmark measures the additional time introduced by each level.

The intended pipeline covers:

1. Source checkout
2. Dependency installation
3. Automated tests
4. Docker image build
5. Image publishing
6. Security operations appropriate to the benchmark level
7. Kubernetes deployment

The exact execution order must remain consistent with the finalized benchmark methodology.

### SBOM and Vulnerability Scanning

For each application component, the benchmark should generate and scan:

- A **source SBOM**, including source files and dependency manifests or lockfiles.
- A **container image SBOM**, representing the final built image.

These are separate artifacts and must not be treated as interchangeable.

### Measurement Requirements

- Measure individual pipeline stages and total execution time.
- Record absolute and relative security overhead.
- Support repeated benchmark executions.
- Report statistics such as median and p95.
- Keep benchmark conditions consistent across security levels.
- Account for caching, runner configuration, and environmental variability.
- Preserve sufficient metadata to reproduce and interpret results.

## Benchmark Integrity

**Reproducibility and measurement validity are critical project requirements.**

Do not introduce changes that artificially influence benchmark results.

In particular:

- Do not add random delays or sleeps.
- Do not add meaningless tests.
- Do not add unused dependencies.
- Do not intentionally introduce vulnerabilities.
- Do not change dependency versions between benchmark levels.
- Do not silently change Docker build behavior or caching assumptions.
- Do not mix unrelated application changes with benchmark comparisons.

Changes affecting dependency graphs, test execution, image contents, or pipeline behavior may affect benchmark results and should be documented.

Once the benchmark workload is finalized, keep the application code and dependency versions fixed throughout the main experiment.

## Guidelines for Coding Agents

Before making changes:

1. Inspect the relevant files and existing implementation.
2. Understand the service's responsibility and integration points.
3. Reuse established project conventions.
4. Make the smallest changes necessary to complete the task.
5. Avoid unrelated refactoring.

After making changes:

1. Run relevant tests.
2. Verify builds when possible.
3. Check integration implications.
4. Summarize modified files and important decisions.
5. Clearly identify anything that could not be verified.

Never claim that tests, Docker builds, or integration checks passed unless they were actually executed successfully.

## Final Principle

**This project prioritizes realistic workloads, simple architecture, reproducible builds, and reliable CI/CD security measurements.**

Every implementation decision should support these goals without introducing unnecessary complexity.
