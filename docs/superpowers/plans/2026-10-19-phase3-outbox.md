# Phase 3 Outbox and Worker Plan

**Goal:** Persist `UserRegistered` in the same transaction as registration, then relay it through an in-process event bus with at-least-once delivery.

## Design

- Identity owns `identity.outbox` and its migration. An application event port writes an envelope with UUIDv7 ID, aggregate ID, version, request/trace context, occurrence time, and JSON payload.
- The relay claims rows in a database transaction with `FOR UPDATE SKIP LOCKED`. It publishes while holding the claim, marks success in the same transaction, and records bounded retry with backoff or dead status on failure.
- The bus accepts handlers by event type and returns a publish error when no handler exists, leaving the outbox row retryable until a consumer is installed in Phase 4.
- `cmd/worker` runs the relay loop. The API process also runs the relay loop in default single-process mode.

## Steps and gates

1. Add event envelope, in-process bus, and tests for dispatch and missing subscribers.
2. Add migration and identity outbox writer; extend registration transaction and test rollback behavior.
3. Add relay with database integration test proving two concurrent relays claim each row once; test retry and dead transition.
4. Wire worker and API lifecycle. Run `make fmt lint test build`, integration tests, and Compose smoke. Update progress and commit one Conventional Commit.
