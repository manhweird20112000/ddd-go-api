# Progress

## Phase 0 — approved

- Design approved on 2026-10-19.
- Go toolchain installed locally: 1.26.3. The `go.mod` directive will use `go 1.26.3`.
- The default Compose mode will run the API and worker loops in one process. Distributed mode will use separate identity and notification processes and NATS JetStream.
- Outbox delivery is at least once. Concurrent relays must not claim the same row; a crash after publish can cause redelivery.
- Consumer database effects are idempotent. The logging Sender cannot guarantee an exactly once external side effect after a crash.
- The root `compose.yaml` will support the required `docker compose up` command.
- The Phase 0 quality gate is design approval because no Makefile or code existed before Phase 1.

## Phase 1 — complete

- Initialized `github.com/yourorg/go-template` with Go 1.26.3, developer commands, lint configuration, environment example, and a CI skeleton.
- Implemented validated configuration, JSON logging, OTLP tracing and metrics, Prometheus scraping, pgx pool and transaction transport, request IDs, error envelope, and liveness/readiness routes.
- `make fmt lint test build` passed on 2026-10-19; golangci-lint reported 0 issues.
- `make test-race` passed in the Go Linux container; Windows has no local C compiler, so that target uses Docker locally and runs natively in Linux CI.
- `make up` built and started PostgreSQL, Redis, OTel collector, and API. `/healthz`, `/readyz`, and `/metrics` each returned HTTP 200.
- Worker, migration, sqlc, proto, arch-check, and module generator targets are defined; their implementations follow the phases that introduce the corresponding module or transport.

## Phase 2 — complete

- Implemented identity domain and application ports for registration, login, refresh rotation with family revocation on reuse, current-user lookup, and cursor pagination.
- Added HTTP, PostgreSQL/sqlc, Argon2id, and Ed25519 JWT adapters. Identity owns its schema and migration history.
- `make fmt lint test build` passed on 2026-10-19; golangci-lint reported 0 issues. A Testcontainers integration test passed against PostgreSQL 16, covering migration, reads, uniqueness, and list projection.
- `docker compose up --build -d` completed migration before starting API. HTTP smoke confirmed registration, login, current-user lookup, paginated list, and refresh rotation.

## Phase 3 — complete

- Identity registration now inserts `UserRegisteredV1` into `identity.outbox` in the same database transaction as the user. The payload carries only `user_id`.
- Added a typed event envelope, in-process bus, worker entrypoint, `SKIP LOCKED` relay, bounded retries, and dead status after ten failures. An event without a subscriber stays pending until a consumer is installed.
- `make test-integration` passed against PostgreSQL 16, including rollback when outbox insert fails, two concurrent relays publishing each of twenty events once, and dead transition. Compose migration completed and registration created one pending event.
- `make fmt lint test build` passed on 2026-10-19 with zero lint issues.

## Phase 4 — complete

- Added Notification domain, application, Postgres/sqlc and in-process adapters, a logging Sender, and module-owned migrations for `processed_events`, `welcome_deliveries`, and outbox.
- The consumer inserts its processed marker and delivery in one transaction. Duplicate source events are acknowledged without another delivery. A sender failure rolls back the transaction for retry.
- `make test-integration` passed with duplicate delivery and two concurrent relays. Compose with two worker containers processed a registration, then the same event was forced to redeliver; its outbox returned to `published` and only one processed marker and welcome row existed.
- `make fmt lint test build` passed on 2026-10-19 with zero lint issues.

## Phases 5–8 — pending

- Phase 5: identity contract, in-process adapter, and architecture enforcement.
- Phase 6: gRPC, JetStream, registry, and distributed smoke test.
- Phase 7: HTTP hardening, separate database roles, and cleanup jobs.
- Phase 8: scaffolder, complete CI, ADRs, and operational documentation.

## Environment findings

- The repository started empty and without Git; Git was initialized for the phase commits.
- `make`, `docker`, `git`, and `buf` are installed. `golangci-lint`, `sqlc`, and `govulncheck` are not currently on PATH.
- Docker Engine requires elevated tool execution in this environment; the Phase 1 Compose acceptance checks passed with that access.
