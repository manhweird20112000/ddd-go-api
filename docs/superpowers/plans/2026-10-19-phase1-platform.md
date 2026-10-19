# Phase 1 Platform Implementation Plan

**Goal:** Build a runnable Go platform skeleton whose default Compose mode serves liveness and readiness endpoints, and whose formatting, lint, tests, and build gates pass.

**Architecture:** `internal/app` owns lifecycle and wiring. `internal/platform` provides configuration, logging, observability, PostgreSQL access, and HTTP primitives without importing modules. No example business module is implemented in this phase.

**Tech stack:** Go 1.26.3, chi v5, pgx/v5, caarlos0/env, OpenTelemetry, Prometheus, slog, errgroup, PostgreSQL 16+, Redis, Docker Compose, golangci-lint.

## Global constraints

- Module path: `github.com/yourorg/go-template`.
- Ports: HTTP 8080, gRPC 9090, metrics 9100.
- Every I/O function takes `context.Context` first.
- Configuration fails fast and secrets come only from environment variables.
- `make fmt lint test build` is the mandatory phase gate.
- Phase 1 implementation stays inside platform and composition root; identity and notification start in later phases.

## Tasks

### Task 1: Module and developer commands

Create `go.mod`, `go.sum`, `Makefile`, `.golangci.yml`, and `.env.example`. Resolve the latest dependency versions compatible with Go 1.26.3 and verify called APIs with `go doc`. Provide every required Make target; targets for later phases may invoke concrete generators or phase-specific commands only when their inputs exist, and must fail clearly otherwise. Verify with `go mod tidy`, `make help`, and `go vet ./...`.

### Task 2: Configuration and server behavior

Create `internal/platform/config`, `logging`, `httpx`, and `internal/app`. First add tests for invalid configuration, request ID behavior, health, readiness, and error response shape; confirm the tests fail before writing implementation. Implement explicit dependency injection, JSON logs, and an HTTP server with timeouts. Verify with `go test ./internal/platform/... ./internal/app/...`.

### Task 3: Database and observability

Create `internal/platform/db`, `tracing`, and `metrics`. Test transaction commit/rollback behavior where a local integration environment is available; test configuration validation without requiring external services. Add pgxpool startup checks, OTLP trace/metric exporters, and a Prometheus scrape server. Verify with `go test ./...` and `go vet ./...`.

### Task 4: Entrypoints and local stack

Create `cmd/api`, `compose.yaml`, `deploy/Dockerfile`, and a CI skeleton. The API starts HTTP and metrics endpoints; readiness checks only enabled dependencies. The worker and migration entrypoints start in the phases that provide their actual work. Verify with `go build ./cmd/...`, then `docker compose up` and HTTP checks if Docker Engine is available.

### Task 5: Gate and phase record

Run `make fmt lint test build`; repair failures and rerun the complete command. Update `docs/PROGRESS.md` with exact verification results and known constraints. Commit Phase 1 once using a Conventional Commit message. Do not mark Phase 1 complete unless the full gate passes.

## Review focus

- Startup and shutdown order, including cancellation and pool closure.
- Readiness semantics and timeout handling.
- No accidental logging of database URLs or secrets.
- Package import direction and future extraction boundary.
