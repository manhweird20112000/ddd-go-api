# Phase 7 Hardening Plan

**Goal:** Add durable write idempotency, Redis rate limiting, HTTP security/CORS, per-module database roles, and retention jobs.

## Design decisions

- Every identity POST requires `Idempotency-Key`. A database transaction covers the identity write and encrypted response record. Request identity is HMACed; response bodies containing refresh tokens are AES-GCM encrypted using a 32-byte env secret. The same key and request return the same response; a changed request conflicts.
- PostgreSQL owns the idempotency table with a 24-hour TTL. The transaction manager joins an existing transport transaction so app writes and the response record commit together.
- Redis fixed-window counters provide per-IP write rate limiting across replicas. A Redis failure rejects auth writes; readiness also reports Redis failure.
- HTTP applies body caps, security headers, and exact-origin CORS from env.
- Identity and Notification use distinct runtime PostgreSQL URLs and roles, with `DATABASE_URL` as local development fallback. Compose initializes roles and schema privileges from environment secrets.
- Cleanup runs periodically in a worker or default API process. Published outbox and processed markers are retained longer than the JetStream replay window; dead-letter rows are retained for inspection.

## Steps and gates

1. Add transactional idempotency migration, encrypted middleware, and tests for replay, conflict, concurrency, and rollback.
2. Add Redis rate limiting, CORS, security headers, and request-size tests.
3. Split module database pools and Compose roles, then add retention jobs.
4. Run `make fmt lint test build`, integration tests, and both Compose smoke modes. Update progress and commit one Conventional Commit.
