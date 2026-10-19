# Progress

## Phase 0 — approved

- Design approved on 2026-10-19.
- Go toolchain installed locally: 1.26.3. The `go.mod` directive will use `go 1.26.3`.
- The default Compose mode will run the API and worker loops in one process. Distributed mode will use separate identity and notification processes and NATS JetStream.
- Outbox delivery is at least once. Concurrent relays must not claim the same row; a crash after publish can cause redelivery.
- Consumer database effects are idempotent. The logging Sender cannot guarantee an exactly once external side effect after a crash.
- The root `compose.yaml` will support the required `docker compose up` command.
- The Phase 0 quality gate is design approval because no Makefile or code existed before Phase 1.

## Phase 1 — in progress

- Initialize the Go module and core tooling.
- Implement validated configuration, JSON logging, observability, DB pool and transaction manager, basic HTTP server, error mapping, request IDs, and health endpoints.
- Provide local Compose dependencies and a CI skeleton.
- Run `make fmt lint test build` and verify health/readiness before marking this phase complete.

## Phases 2–8 — pending

- Phase 2: identity HTTP and Postgres flow.
- Phase 3: transactional outbox, in-process event bus, and relay.
- Phase 4: notification consumer and idempotent database effect.
- Phase 5: identity contract, in-process adapter, and architecture enforcement.
- Phase 6: gRPC, JetStream, registry, and distributed smoke test.
- Phase 7: HTTP hardening, separate database roles, and cleanup jobs.
- Phase 8: scaffolder, complete CI, ADRs, and operational documentation.

## Environment findings

- The repository started empty and without Git; Git was initialized for the phase commits.
- `make`, `docker`, `git`, and `buf` are installed. `golangci-lint`, `sqlc`, and `govulncheck` are not currently on PATH.
- Docker Engine is not accessible from the current sandbox. This must be resolved before the Compose acceptance checks can run.
