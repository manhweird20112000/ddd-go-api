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

## Phase 2 — next

- Implement the identity domain, application, HTTP and PostgreSQL adapters, migrations, password hashing, and token flow.
- Wire `/v1` identity routes into the API composition root and verify register, login, and me with curl.

## Phases 3–8 — pending

- Phase 3: transactional outbox, in-process event bus, and relay.
- Phase 4: notification consumer and idempotent database effect.
- Phase 5: identity contract, in-process adapter, and architecture enforcement.
- Phase 6: gRPC, JetStream, registry, and distributed smoke test.
- Phase 7: HTTP hardening, separate database roles, and cleanup jobs.
- Phase 8: scaffolder, complete CI, ADRs, and operational documentation.

## Environment findings

- The repository started empty and without Git; Git was initialized for the phase commits.
- `make`, `docker`, `git`, and `buf` are installed. `golangci-lint`, `sqlc`, and `govulncheck` are not currently on PATH.
- Docker Engine requires elevated tool execution in this environment; the Phase 1 Compose acceptance checks passed with that access.
