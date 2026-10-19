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

## Phase 5 — complete

- Identity now exposes `contract.Service.GetUser` through an in-process adapter. Notification resolves the email by ID through a consumer-owned lookup port and stores it as delivery recipient; event payload remains `user_id` only.
- Added `go/packages` architecture tests for cross-module contract imports, stdlib-only domains, application infrastructure isolation, and platform independence. Depguard has module-specific rules.
- Deliberately imported `identity/domain` from Notification during validation: both `make arch-check` and `make lint` failed, then passed after removing the probe.
- `make test-integration` passed. Compose with API and two worker containers processed registration and stored the email obtained through the local Identity contract.
- `make fmt lint test build` passed on 2026-10-19 with zero lint issues.

## Phase 6 — complete

- Added protobuf/buf generation, Identity gRPC server and contract client, a 2-second default deadline, retries with jitter for idempotent `GetUser`, and a circuit breaker. One contract suite passed for local and gRPC adapters.
- Added a durable JetStream publisher/consumer with explicit acknowledgements, delayed retries, dead-letter subject, and message-ID deduplication. Composition selects in-process or NATS transport and local or remote Identity contract from environment configuration.
- `docker compose --profile distributed up --build -d` started Identity API, Notification worker, and NATS. With the old monolith services stopped, a registration on port 8081 produced a published outbox row and a Notification delivery whose recipient matched Identity's email through gRPC. Request and trace IDs matched across HTTP and worker logs.
- `make fmt lint test build`, `make arch-check`, `buf lint`, and `make proto` passed on 2026-10-19.
- Compose profiles include unprofiled base services by design; a pure distributed run uses `docker compose --profile distributed up --build -d --scale api=0`.

## Phase 7 — complete

- Identity writes require `Idempotency-Key`. PostgreSQL stores a request HMAC and AES-GCM encrypted response in the same transaction as the write for 24 hours. Integration tests cover replay, changed payloads, concurrent duplicates, and rollback on a 5xx response.
- Redis enforces 20 writes per minute per client IP and route across replicas; Redis failure rejects writes. HTTP adds a 1 MiB body cap, security headers, and exact-origin CORS from environment configuration.
- Identity and Notification open separate runtime PostgreSQL pools. Compose creates `identity_app` and `notification_app` roles, grants each only its own schema, and runs admin migrations separately. A privilege query returned `t|f|t|f` for own and cross-module table reads.
- Bounded hourly cleanup removes expired idempotency keys, published outbox rows after 45 days, dead outbox rows after 90 days, and processed event markers after 45 days. JetStream retains events for 30 days. Shutdown drains servers and consumers before a final relay flush.
- `make fmt lint test build` and `make test-integration` passed on 2026-10-19 with zero lint issues. Distributed Compose returned 201 for registration and identical replay, 409 for a changed payload, and 429 after 20 login requests. Monolith Compose registration produced one Notification delivery.

## Phase 8 — pending

- Scaffolder, complete CI, ADRs, and operational documentation.

## Environment findings

- The repository started empty and without Git; Git was initialized for the phase commits.
- `make`, `docker`, `git`, and `buf` are installed. `golangci-lint`, `sqlc`, and `govulncheck` are not currently on PATH.
- Docker Engine requires elevated tool execution in this environment; the Phase 1 Compose acceptance checks passed with that access.
