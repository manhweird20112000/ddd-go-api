# Phase 2 Identity Implementation Plan

**Goal:** Add working user registration, login, refresh-token rotation, current-user lookup, and cursor-paginated user listing over PostgreSQL and HTTP.

**Architecture:** Identity owns its domain, application ports, HTTP and Postgres adapters, migrations, and sqlc output. The composition root injects a pgx pool, password hasher, and Ed25519 JWT issuer. All writes use the application unit-of-work port.

**Tech stack:** pgx/v5, sqlc, golang-migrate pgx/v5 driver, golang-jwt/jwt/v5, Argon2id, go-playground/validator, testcontainers-go, Go 1.26.3.

## Global constraints

- Domain imports only the Go standard library; app imports no pgx, chi, or gRPC.
- PostgreSQL data lives in `identity` schema; migration version table is `identity.schema_migrations`.
- Refresh tokens are opaque random values; only SHA-256 hashes are stored.
- JWT signing key comes from `JWT_PRIVATE_KEY_BASE64` and is never logged.
- Errors are wrapped internally and mapped once at the HTTP boundary.
- `make fmt lint test build` and register/login/me curl flow gate completion.

## Tasks

### Task 1: Schema and generated queries

Add identity migration and `sqlc.yaml`, generate pgx/v5 query code, and add `cmd/migrate` that uses golang-migrate with an embedded file source and per-schema migration table. Verify migration up/down on the Compose PostgreSQL instance and check table placement.

### Task 2: Domain and application

Define user and refresh-token domain state, sentinel errors, and small repository, unit-of-work, hasher, and token ports. Write failing tests for register validation/conflict, login invalid credentials, refresh rotation/reuse, and cursor validation. Implement the command and query handlers, then run package tests.

### Task 3: Adapters and HTTP

Implement Postgres repositories using sqlc, Argon2id hashing, Ed25519 JWT signing and verification, and Chi HTTP routes. Wire them in `internal/app`. Add handler tests for request/response/status behavior. Verify with `go test ./internal/modules/identity/...`.

### Task 4: Integration and acceptance

Add repository integration coverage with testcontainers, update Compose and documentation, and run `make fmt lint test build`. Run migration and curl register, login, me, refresh, and list against Compose. Update `docs/PROGRESS.md` with results and commit one Conventional Commit for Phase 2.
